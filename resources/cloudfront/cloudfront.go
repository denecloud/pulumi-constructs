package cloudfront

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cloudfront"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CloudFrontConfig holds the configuration for the CloudFront distribution
type CloudFrontConfig struct {
	// Domain configuration
	DomainName     string
	Aliases        []string
	CertificateArn string // Optional: if not provided and Aliases are set, will create new cert

	// Origin configuration
	OriginDomain         string
	OriginPath           string
	OriginProtocolPolicy string // Optional: defaults to "https-only"

	// Cache configuration
	DefaultTTL int    // Optional: defaults to 86400 (1 day)
	MaxTTL     int    // Optional: defaults to 31536000 (1 year)
	MinTTL     int    // Optional: defaults to 0
	PriceClass string // Optional: defaults to "PriceClass_100"

	// Security configuration
	ViewerProtocolPolicy string // Optional: defaults to "redirect-to-https"
	WAFWebACLID          string // Optional: WAF Web ACL ID to associate

	// General configuration
	Enabled     bool              // Optional: defaults to true
	IPV6Enabled bool              // Optional: defaults to true
	Tags        map[string]string // Optional: resource tags
	Environment string            // Required: deployment environment
}

// CloudFrontDistribution is a custom component that creates a CloudFront distribution
type CloudFrontDistribution struct {
	pulumi.ComponentResource

	// Exported fields
	Distribution    *cloudfront.Distribution
	DomainName      pulumi.StringOutput
	DistributionID  pulumi.StringOutput
	DistributionArn pulumi.StringOutput
}

// NewCloudFrontDistribution creates a new CloudFront distribution component
func NewCloudFrontDistribution(ctx *pulumi.Context, name string, config *CloudFrontConfig, opts ...pulumi.ResourceOption) (*CloudFrontDistribution, error) {
	comp := &CloudFrontDistribution{}

	// Initialize the component resource
	err := ctx.RegisterComponentResource("custom:aws:CloudFrontDistribution", name, comp, opts...)
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
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 86400
	}
	if config.MaxTTL == 0 {
		config.MaxTTL = 31536000
	}
	if config.PriceClass == "" {
		config.PriceClass = "PriceClass_100"
	}
	if config.ViewerProtocolPolicy == "" {
		config.ViewerProtocolPolicy = "redirect-to-https"
	}
	if config.OriginProtocolPolicy == "" {
		config.OriginProtocolPolicy = "https-only"
	}

	// Create origin configuration
	origin := cloudfront.DistributionOriginArgs{
		DomainName: pulumi.String(config.OriginDomain),
		OriginPath: pulumi.String(config.OriginPath),
		CustomOriginConfig: &cloudfront.DistributionOriginCustomOriginConfigArgs{
			OriginProtocolPolicy: pulumi.String(config.OriginProtocolPolicy),
			HTTPPort:             pulumi.Int(80),
			HTTPSPort:            pulumi.Int(443),
			OriginSslProtocols:   pulumi.StringArray{pulumi.String("TLSv1.2")},
		},
	}

	// Create default cache behavior
	defaultCacheBehavior := cloudfront.DistributionDefaultCacheBehaviorArgs{
		TargetOriginId:       pulumi.String("primary"),
		ViewerProtocolPolicy: pulumi.String(config.ViewerProtocolPolicy),
		AllowedMethods: pulumi.StringArray{
			pulumi.String("GET"),
			pulumi.String("HEAD"),
			pulumi.String("OPTIONS"),
		},
		CachedMethods: pulumi.StringArray{
			pulumi.String("GET"),
			pulumi.String("HEAD"),
		},
		ForwardedValues: &cloudfront.DistributionDefaultCacheBehaviorForwardedValuesArgs{
			QueryString: pulumi.Bool(true),
			Cookies: &cloudfront.DistributionDefaultCacheBehaviorForwardedValuesCookiesArgs{
				Forward: pulumi.String("none"),
			},
		},
		MinTTL:     pulumi.Int(config.MinTTL),
		DefaultTTL: pulumi.Int(config.DefaultTTL),
		MaxTTL:     pulumi.Int(config.MaxTTL),
		Compress:   pulumi.Bool(true),
	}

	// Create the CloudFront distribution
	distribution, err := cloudfront.NewDistribution(ctx, name, &cloudfront.DistributionArgs{
		Enabled:       pulumi.Bool(config.Enabled),
		IsIPV6Enabled: pulumi.Bool(config.IPV6Enabled),
		PriceClass:    pulumi.String(config.PriceClass),
		Aliases:       pulumi.ToStringArray(config.Aliases),
		Tags:          pulumi.ToStringMap(tags),
		WebACLId:      pulumi.String(config.WAFWebACLID),

		Origins: cloudfront.DistributionOriginArray{
			origin,
		},

		DefaultCacheBehavior: defaultCacheBehavior,

		Restrictions: &cloudfront.DistributionRestrictionsArgs{
			GeoRestriction: &cloudfront.DistributionRestrictionsGeoRestrictionArgs{
				RestrictionType: pulumi.String("none"),
			},
		},

		ViewerCertificate: &cloudfront.DistributionViewerCertificateArgs{
			AcmCertificateArn:      pulumi.String(config.CertificateArn),
			MinimumProtocolVersion: pulumi.String("TLSv1.2_2021"),
			SslSupportMethod:       pulumi.String("sni-only"),
		},
	}, &parentOpts)

	if err != nil {
		return nil, fmt.Errorf("failed to create distribution: %w", err)
	}

	// Store the distribution and outputs
	comp.Distribution = distribution
	comp.DomainName = distribution.DomainName
	comp.DistributionID = distribution.ID()
	comp.DistributionArn = distribution.Arn

	return comp, nil
}
