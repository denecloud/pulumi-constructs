Example

```
func main() {
    pulumi.Run(func(ctx *pulumi.Context) error {
        cdn, err := NewCloudFrontDistribution(ctx, "my-cdn", &CloudFrontConfig{
            DomainName:    "my-app.example.com",
            Aliases:       []string{"my-app.example.com"},
            OriginDomain:  "origin.example.com",
            Environment:   "production",
            PriceClass:    "PriceClass_100",
            Tags: map[string]string{
                "Team": "Platform",
            },
        })
        if err != nil {
            return err
        }

        // Export values
        ctx.Export("distributionId", cdn.DistributionID)
        ctx.Export("domainName", cdn.DomainName)
        return nil
    })
}
```