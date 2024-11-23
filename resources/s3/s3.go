package s3

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// BucketConfig holds the configuration for the S3 bucket
type BucketConfig struct {
	// BucketName is the name of the S3 bucket
	BucketName string
	// Environment is the deployment environment (dev, staging, prod)
	Environment string
	// Tags to be applied to all resources
	Tags map[string]string
}

// SecureBucket is a custom component that creates an S3 bucket with security best practices
type SecureBucket struct {
	pulumi.ComponentResource

	// Exported fields
	Bucket    *s3.Bucket
	BucketArn pulumi.StringOutput
}

// NewSecureBucket creates a new SecureBucket component
func NewSecureBucket(ctx *pulumi.Context, name string, config *BucketConfig, opts ...pulumi.ResourceOption) (*SecureBucket, error) {
	comp := &SecureBucket{}

	// Initialize the component resource
	parentOpts := pulumi.ResourceOptions{
		Parent: comp,
	}
	err := ctx.RegisterComponentResource("custom:aws:SecureBucket", name, comp, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to register component: %w", err)
	}

	// Merge default tags with provided tags
	tags := map[string]string{
		"Environment": config.Environment,
		"ManagedBy":   "Pulumi",
	}
	for k, v := range config.Tags {
		tags[k] = v
	}

	// Create the S3 bucket with best practices
	bucket, err := s3.NewBucket(ctx, name, &s3.BucketArgs{
		Bucket: pulumi.String(config.BucketName),
		Tags:   pulumi.ToStringMap(tags),

		// Security best practices
		ServerSideEncryptionConfiguration: &s3.BucketServerSideEncryptionConfigurationArgs{
			Rule: &s3.BucketServerSideEncryptionConfigurationRuleArgs{
				ApplyServerSideEncryptionByDefault: &s3.BucketServerSideEncryptionConfigurationRuleApplyServerSideEncryptionByDefaultArgs{
					SseAlgorithm: pulumi.String("AES256"),
				},
			},
		},
		Versioning: &s3.BucketVersioningArgs{
			Enabled: pulumi.Bool(true),
		},
		BlockPublicAcls:       pulumi.Bool(true),
		BlockPublicPolicy:     pulumi.Bool(true),
		IgnorePublicAcls:      pulumi.Bool(true),
		RestrictPublicBuckets: pulumi.Bool(true),
	}, &parentOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	// Store the bucket and its ARN
	comp.Bucket = bucket
	comp.BucketArn = bucket.Arn

	return comp, nil
}
