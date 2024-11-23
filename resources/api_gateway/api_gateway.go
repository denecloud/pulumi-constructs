package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/apigateway"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/lambda"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// APIGatewayConfig holds the configuration for the API Gateway
type APIGatewayConfig struct {
	// API configuration
	Name        string
	Description string
	StageName   string // e.g., "prod", "dev"

	// Security configuration
	EnableCORS     bool
	AuthorizerFunc *lambda.Function // Optional: Lambda authorizer
	ApiKeyRequired bool
	UsagePlanLimit *UsagePlanConfig

	// Domain configuration
	CustomDomain *CustomDomainConfig

	// Endpoint configuration
	Endpoints []EndpointConfig

	// General configuration
	Tags        map[string]string
	Environment string // Required: deployment environment
}

// CustomDomainConfig holds custom domain configuration
type CustomDomainConfig struct {
	DomainName     string
	CertificateArn string
	Route53ZoneId  string // Optional: if you want automatic DNS setup
}

// UsagePlanConfig defines API usage limits
type UsagePlanConfig struct {
	Quota    *QuotaConfig
	Throttle *ThrottleConfig
}

// QuotaConfig defines quota limits
type QuotaConfig struct {
	Limit  int
	Period string // "DAY", "WEEK", or "MONTH"
}

// ThrottleConfig defines throttling limits
type ThrottleConfig struct {
	BurstLimit int
	RateLimit  float64
}

// EndpointConfig defines an API endpoint
type EndpointConfig struct {
	Path              string // e.g., "/users"
	Method            string // GET, POST, etc.
	LambdaFunc        *lambda.Function
	Authorization     string // "NONE", "AWS_IAM", "CUSTOM"
	ApiKeyRequired    bool
	RequestParameters map[string]bool
	RequestModels     map[string]string
}

// APIGateway is a custom component that creates an API Gateway with associated resources
type APIGateway struct {
	pulumi.ComponentResource

	// Exported fields
	RestAPI         *apigateway.RestApi
	Deployment      *apigateway.Deployment
	Stage           *apigateway.Stage
	APIKey          *apigateway.ApiKey
	UsagePlan       *apigateway.UsagePlan
	CustomDomain    *apigateway.DomainName
	BaseURL         pulumi.StringOutput
	CustomDomainURL pulumi.StringOutput
}

// NewAPIGateway creates a new API Gateway component
func NewAPIGateway(ctx *pulumi.Context, name string, config *APIGatewayConfig, opts ...pulumi.ResourceOption) (*APIGateway, error) {
	comp := &APIGateway{}

	// Initialize the component resource
	err := ctx.RegisterComponentResource("custom:aws:APIGateway", name, comp, opts...)
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

	// Create the REST API
	restAPI, err := apigateway.NewRestApi(ctx, name, &apigateway.RestApiArgs{
		Name:        pulumi.String(config.Name),
		Description: pulumi.String(config.Description),
		EndpointConfiguration: &apigateway.RestApiEndpointConfigurationArgs{
			Types: pulumi.StringArray{pulumi.String("EDGE")},
		},
		Tags: pulumi.ToStringMap(tags),
	}, &parentOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST API: %w", err)
	}

	// Create Lambda authorizer if provided
	var authorizer *apigateway.Authorizer
	if config.AuthorizerFunc != nil {
		authorizer, err = apigateway.NewAuthorizer(ctx, name+"-authorizer", &apigateway.AuthorizerArgs{
			RestApi:                      restAPI.ID(),
			Name:                         pulumi.String(name + "-authorizer"),
			Type:                         pulumi.String("TOKEN"),
			AuthorizerUri:                config.AuthorizerFunc.InvokeArn,
			IdentitySource:               pulumi.String("method.request.header.Authorization"),
			AuthorizerResultTtlInSeconds: pulumi.Int(300),
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create authorizer: %w", err)
		}
	}

	// Create resources and methods for each endpoint
	resources := make(map[string]*apigateway.Resource)
	for _, endpoint := range config.Endpoints {
		// Create or get resource
		path := endpoint.Path
		parentPath := "/"
		resource := restAPI.RootResourceId

		// Split path and create resources hierarchically
		for _, pathPart := range splitPath(path) {
			if pathPart == "" {
				continue
			}

			fullPath := parentPath + pathPart
			if existing, ok := resources[fullPath]; ok {
				resource = existing.ID()
			} else {
				newResource, err := apigateway.NewResource(ctx, name+"-"+pathPart, &apigateway.ResourceArgs{
					RestApi:  restAPI.ID(),
					ParentId: resource,
					PathPart: pulumi.String(pathPart),
				}, &parentOpts)
				if err != nil {
					return nil, fmt.Errorf("failed to create resource for path %s: %w", fullPath, err)
				}
				resources[fullPath] = newResource
				resource = newResource.ID()
			}
			parentPath = fullPath + "/"
		}

		// Create method
		methodArgs := &apigateway.MethodArgs{
			RestApi:        restAPI.ID(),
			ResourceId:     resource,
			HttpMethod:     pulumi.String(endpoint.Method),
			Authorization:  pulumi.String(endpoint.Authorization),
			ApiKeyRequired: pulumi.Bool(endpoint.ApiKeyRequired),
		}

		if authorizer != nil && endpoint.Authorization == "CUSTOM" {
			methodArgs.AuthorizerId = authorizer.ID()
		}

		if endpoint.RequestParameters != nil {
			methodArgs.RequestParameters = pulumi.BoolMap{}
			for k, v := range endpoint.RequestParameters {
				methodArgs.RequestParameters[k] = pulumi.Bool(v)
			}
		}

		method, err := apigateway.NewMethod(ctx, name+"-"+endpoint.Method+"-"+path, methodArgs, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create method: %w", err)
		}

		// Create integration
		integration, err := apigateway.NewIntegration(ctx, name+"-"+endpoint.Method+"-"+path+"-integration", &apigateway.IntegrationArgs{
			RestApi:               restAPI.ID(),
			ResourceId:            resource,
			HttpMethod:            method.HttpMethod,
			IntegrationType:       pulumi.String("AWS_PROXY"),
			IntegrationHttpMethod: pulumi.String("POST"),
			Uri:                   endpoint.LambdaFunc.InvokeArn,
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create integration: %w", err)
		}

		// Add permission to Lambda
		_, err = lambda.NewPermission(ctx, name+"-"+endpoint.Method+"-"+path+"-permission", &lambda.PermissionArgs{
			Action:    pulumi.String("lambda:InvokeFunction"),
			Function:  endpoint.LambdaFunc.Name,
			Principal: pulumi.String("apigateway.amazonaws.com"),
			SourceArn: pulumi.Sprintf("arn:aws:execute-api:%s:%s:%s/*/%s%s",
				ctx.Region(),
				ctx.Account(),
				restAPI.ID(),
				endpoint.Method,
				endpoint.Path),
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create lambda permission: %w", err)
		}

		// Add CORS if enabled
		if config.EnableCORS {
			_, err = apigateway.NewMethod(ctx, name+"-"+endpoint.Method+"-"+path+"-options", &apigateway.MethodArgs{
				RestApi:        restAPI.ID(),
				ResourceId:     resource,
				HttpMethod:     pulumi.String("OPTIONS"),
				Authorization:  pulumi.String("NONE"),
				ApiKeyRequired: pulumi.Bool(false),
			}, &parentOpts)
			if err != nil {
				return nil, fmt.Errorf("failed to create OPTIONS method: %w", err)
			}

			_, err = apigateway.NewIntegration(ctx, name+"-"+endpoint.Method+"-"+path+"-options-integration", &apigateway.IntegrationArgs{
				RestApi:         restAPI.ID(),
				ResourceId:      resource,
				HttpMethod:      pulumi.String("OPTIONS"),
				IntegrationType: pulumi.String("MOCK"),
				RequestTemplates: pulumi.StringMap{
					"application/json": pulumi.String(`{"statusCode": 200}`),
				},
			}, &parentOpts)
			if err != nil {
				return nil, fmt.Errorf("failed to create OPTIONS integration: %w", err)
			}
		}
	}

	// Create deployment
	deployment, err := apigateway.NewDeployment(ctx, name+"-deployment", &apigateway.DeploymentArgs{
		RestApi: restAPI.ID(),
		Triggers: pulumi.StringMap{
			"redeployment": pulumi.String(fmt.Sprintf("%v", config.Endpoints)),
		},
	}, &parentOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	// Create stage
	stage, err := apigateway.NewStage(ctx, name+"-stage", &apigateway.StageArgs{
		RestApi:    restAPI.ID(),
		Deployment: deployment.ID(),
		StageName:  pulumi.String(config.StageName),
		Tags:       pulumi.ToStringMap(tags),
	}, &parentOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create stage: %w", err)
	}

	// Create API key and usage plan if required
	var apiKey *apigateway.ApiKey
	var usagePlan *apigateway.UsagePlan
	if config.ApiKeyRequired && config.UsagePlanLimit != nil {
		apiKey, err = apigateway.NewApiKey(ctx, name+"-key", &apigateway.ApiKeyArgs{
			Name: pulumi.String(name + "-key"),
			Tags: pulumi.ToStringMap(tags),
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create API key: %w", err)
		}

		usagePlan, err = apigateway.NewUsagePlan(ctx, name+"-usage-plan", &apigateway.UsagePlanArgs{
			Name: pulumi.String(name + "-usage-plan"),
			ApiStages: apigateway.UsagePlanApiStageArray{
				&apigateway.UsagePlanApiStageArgs{
					ApiId: restAPI.ID(),
					Stage: stage.StageName,
				},
			},
			Quota: &apigateway.UsagePlanQuotaArgs{
				Limit:  pulumi.Int(config.UsagePlanLimit.Quota.Limit),
				Period: pulumi.String(config.UsagePlanLimit.Quota.Period),
			},
			Throttle: &apigateway.UsagePlanThrottleArgs{
				BurstLimit: pulumi.Int(config.UsagePlanLimit.Throttle.BurstLimit),
				RateLimit:  pulumi.Float64(config.UsagePlanLimit.Throttle.RateLimit),
			},
			Tags: pulumi.ToStringMap(tags),
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create usage plan: %w", err)
		}

		_, err = apigateway.NewUsagePlanKey(ctx, name+"-usage-plan-key", &apigateway.UsagePlanKeyArgs{
			KeyId:       apiKey.ID(),
			KeyType:     pulumi.String("API_KEY"),
			UsagePlanId: usagePlan.ID(),
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create usage plan key: %w", err)
		}
	}

	// Create custom domain if configured
	var customDomain *apigateway.DomainName
	if config.CustomDomain != nil {
		customDomain, err = apigateway.NewDomainName(ctx, name+"-domain", &apigateway.DomainNameArgs{
			DomainName:     pulumi.String(config.CustomDomain.DomainName),
			CertificateArn: pulumi.String(config.CustomDomain.CertificateArn),
			SecurityPolicy: pulumi.String("TLS_1_2"),
			Tags:           pulumi.ToStringMap(tags),
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create custom domain: %w", err)
		}

		_, err = apigateway.NewBasePathMapping(ctx, name+"-domain-mapping", &apigateway.BasePathMappingArgs{
			RestApi:    restAPI.ID(),
			Stage:      stage.StageName,
			DomainName: customDomain.DomainName,
		}, &parentOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create base path mapping: %w", err)
		}
	}

	// Store resources and outputs
	comp.RestAPI = restAPI
	comp.Deployment = deployment
	comp.Stage = stage
	comp.APIKey = apiKey
	comp.UsagePlan = usagePlan
	comp.CustomDomain = customDomain
	comp.BaseURL = pulumi.Sprintf("https://%s.execute-api.%s.amazonaws.com/%s",
		restAPI.ID(), ctx.Region(), stage.StageName)

	if customDomain != nil {
		comp.CustomDomainURL = pulumi.Sprintf("https://%s", customDomain.DomainName)
	}

	return comp, nil
}

// Helper function to split path into parts
func splitPath(path string) []string {
	// Implementation of path splitting logic
	// This would handle paths like "/users/{id}/profile" appropriately
	// You could use strings.Split() and clean up the parts
	return []string{path} // Simplified for brevity
}
