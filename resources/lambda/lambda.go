package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/lambda"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// LambdaConfig holds the configuration for the Lambda function
type LambdaConfig struct {
	// Function configuration
	Runtime     string
	Handler     string
	Code        *lambda.FunctionArchive
	Description string
	MemorySize  int // Optional: defaults to 128
	Timeout     int // Optional: defaults to 3 seconds
	Environment map[string]string

	// VPC Configuration
	VpcConfig *lambda.FunctionVpcConfigArgs

	// Security configuration
	EnableXRay bool // Optional: defaults to true
	LayerARNs  []string

	// Monitoring configuration
	LogRetentionDays int // Optional: defaults to 14
	AlertConfig      *AlertConfig

	// General configuration
	Tags        map[string]string
	Environment string // Required: deployment environment
}

// AlertConfig holds the configuration for Lambda monitoring alerts
type AlertConfig struct {
	ErrorThreshold     float64 // Number of errors to trigger alert
	ThrottlesThreshold float64 // Number of throttles to trigger alert
	DurationThreshold  float64 // Duration threshold in milliseconds
	NotificationARN    string  // SNS topic ARN for notifications
}

// LambdaFunction is a custom component that creates a Lambda function with associated resources
type LambdaFunction struct {
	pulumi.ComponentResource

	// Exported fields
	Function     *lambda.Function
	Role         *iam.Role
	FunctionName pulumi.StringOutput
	FunctionArn  pulumi.StringOutput
	LogGroupName pulumi.StringOutput
	Alias        *lambda.Alias
}

// NewLambdaFunction creates a new Lambda function component
func NewLambdaFunction(ctx *pulumi.Context, name string, config *LambdaConfig, opts ...pulumi.ResourceOption) (*LambdaFunction, error) {
	comp := &LambdaFunction{}

	// Initialize the component resource
	err := ctx.RegisterComponentResource("custom:aws:LambdaFunction", name, comp, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to register component: %w", err)
	}

	// Set default options
	parentOpts := pulumi.ResourceOptions{
		Parent: comp,
	}

	// Merge default tags with provided tags
	tags := map[string]string{
		"Environment": config.Environment,
		"ManagedBy":   "Pulumi",
	}
	for k, v := range config.Tags {
		tags[k] = v
	}

	// Set default values
	if config.MemorySize == 0 {
		config.MemorySize = 128
	}
	if config.Timeout == 0 {
		config.Timeout = 3
	}
	if config.LogRetentionDays == 0 {
		config.LogRetentionDays = 14
	}

	// Create IAM role for Lambda
	rolePolicy := `{
        "Version": "2012-10-17",
        "Statement": [{
            "Action": "sts:AssumeRole",
            "Principal": {
                "Service": "lambda.amazonaws.com"
            },
            "Effect": "Allow"
        }]
    }`

	role, err := iam.NewRole(ctx, name+"-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(rolePolicy),
		Tags:             pulumi.ToStringMap(tags),
	}, &parentOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create role: %w", err)
	}

	// Attach basic execution policy
	_, err = iam.NewRolePolicyAttachment(ctx, name+"-basic", &iam.RolePolicyAttachmentArgs{
		Role:      role.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
	}, &parentOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to attach basic policy: %w", err)
	}

	// Attach X-Ray policy if enabled
	if config.EnableXRay {
		_, err = iam.NewRolePolicyAttachment(ctx, name+"-xray", &iam.RolePolicyAttachmentArgs{
			Role:      role.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AWSXRayDaemonWriteAccess"),
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to attach x-ray policy: %w", err)
		}
	}

	// Attach VPC policy if VPC config is provided
	if config.VpcConfig != nil {
		_, err = iam.NewRolePolicyAttachment(ctx, name+"-vpc", &iam.RolePolicyAttachmentArgs{
			Role:      role.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"),
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to attach vpc policy: %w", err)
		}
	}

	// Create the Lambda function
	function, err := lambda.NewFunction(ctx, name, &lambda.FunctionArgs{
		Role:        role.Arn,
		Runtime:     pulumi.String(config.Runtime),
		Handler:     pulumi.String(config.Handler),
		Code:        config.Code,
		Description: pulumi.String(config.Description),
		MemorySize:  pulumi.Int(config.MemorySize),
		Timeout:     pulumi.Int(config.Timeout),
		Environment: &lambda.FunctionEnvironmentArgs{
			Variables: pulumi.ToStringMap(config.Environment),
		},
		VpcConfig: config.VpcConfig,
		Layers:    pulumi.ToStringArray(config.LayerARNs),
		TracingConfig: &lambda.FunctionTracingConfigArgs{
			Mode: pulumi.String("Active"),
		},
		Tags: pulumi.ToStringMap(tags),
	}, &parentOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create function: %w", err)
	}

	// Create log group with retention
	logGroup, err := cloudwatch.NewLogGroup(ctx, name+"-logs", &cloudwatch.LogGroupArgs{
		Name:            function.Name.ApplyT(func(name string) string { return "/aws/lambda/" + name }).(pulumi.StringOutput),
		RetentionInDays: pulumi.Int(config.LogRetentionDays),
		Tags:            pulumi.ToStringMap(tags),
	}, &parentOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create log group: %w", err)
	}

	// Create alerts if configured
	if config.AlertConfig != nil {
		// Error count alarm
		_, err = cloudwatch.NewMetricAlarm(ctx, name+"-errors", &cloudwatch.MetricAlarmArgs{
			ComparisonOperator: pulumi.String("GreaterThanThreshold"),
			EvaluationPeriods:  pulumi.Int(1),
			MetricName:         pulumi.String("Errors"),
			Namespace:          pulumi.String("AWS/Lambda"),
			Period:             pulumi.Int(300),
			Statistic:          pulumi.String("Sum"),
			Threshold:          pulumi.Float64(config.AlertConfig.ErrorThreshold),
			AlarmDescription:   pulumi.String(fmt.Sprintf("Lambda function %s error count", name)),
			AlarmActions:       pulumi.StringArray{pulumi.String(config.AlertConfig.NotificationARN)},
			Dimensions: pulumi.StringMap{
				"FunctionName": function.Name,
			},
			Tags: pulumi.ToStringMap(tags),
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create error alarm: %w", err)
		}

		// Duration alarm
		_, err = cloudwatch.NewMetricAlarm(ctx, name+"-duration", &cloudwatch.MetricAlarmArgs{
			ComparisonOperator: pulumi.String("GreaterThanThreshold"),
			EvaluationPeriods:  pulumi.Int(1),
			MetricName:         pulumi.String("Duration"),
			Namespace:          pulumi.String("AWS/Lambda"),
			Period:             pulumi.Int(300),
			Statistic:          pulumi.String("Average"),
			Threshold:          pulumi.Float64(config.AlertConfig.DurationThreshold),
			AlarmDescription:   pulumi.String(fmt.Sprintf("Lambda function %s duration", name)),
			AlarmActions:       pulumi.StringArray{pulumi.String(config.AlertConfig.NotificationARN)},
			Dimensions: pulumi.StringMap{
				"FunctionName": function.Name,
			},
			Tags: pulumi.ToStringMap(tags),
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create duration alarm: %w", err)
		}
	}

	// Create production alias
	alias, err := lambda.NewAlias(ctx, name+"-prod", &lambda.AliasArgs{
		Name:            pulumi.String("prod"),
		FunctionName:    function.Name,
		FunctionVersion: pulumi.String("$LATEST"),
	}, &parentOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create alias: %w", err)
	}

	// Store the resources and outputs
	comp.Function = function
	comp.Role = role
	comp.FunctionName = function.Name
	comp.FunctionArn = function.Arn
	comp.LogGroupName = logGroup.Name
	comp.Alias = alias

	return comp, nil
}
