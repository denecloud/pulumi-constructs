Example

```
func main() {
    pulumi.Run(func(ctx *pulumi.Context) error {
        // First, let's create two Lambda functions for our API endpoints
        getUsersFunction, err := NewLambdaFunction(ctx, "get-users", &LambdaConfig{
            Runtime:     "nodejs18.x",
            Handler:     "index.handler",
            Code:        pulumi.NewFileArchive("./handlers/getUsers.zip"),
            Description: "Get users handler",
            Environment: map[string]string{
                "STAGE": "production",
                "TABLE_NAME": "users",
            },
            MemorySize: 256,
            Timeout:    30,
            Environment: "production",
        })
        if err != nil {
            return err
        }

        createUserFunction, err := NewLambdaFunction(ctx, "create-user", &LambdaConfig{
            Runtime:     "nodejs18.x",
            Handler:     "index.handler",
            Code:        pulumi.NewFileArchive("./handlers/createUser.zip"),
            Description: "Create user handler",
            Environment: map[string]string{
                "STAGE": "production",
                "TABLE_NAME": "users",
            },
            MemorySize: 256,
            Timeout:    30,
            Environment: "production",
        })
        if err != nil {
            return err
        }

        // Now create the API Gateway
        api, err := NewAPIGateway(ctx, "users-api", &APIGatewayConfig{
            Name:        "Users API",
            Description: "API for managing users",
            StageName:   "prod",
            EnableCORS:  true,
            
            // Define the API endpoints
            Endpoints: []EndpointConfig{
                {
                    Path:           "/users",
                    Method:         "GET",
                    LambdaFunc:     getUsersFunction.Function,
                    Authorization:  "NONE",
                    ApiKeyRequired: false,
                },
                {
                    Path:           "/users",
                    Method:         "POST",
                    LambdaFunc:     createUserFunction.Function,
                    Authorization:  "NONE",
                    ApiKeyRequired: true,
                    RequestParameters: map[string]bool{
                        "method.request.header.Content-Type": true,
                    },
                },
            },

            // Optional: Add usage plan limits
            UsagePlanLimit: &UsagePlanConfig{
                Quota: &QuotaConfig{
                    Limit:  1000,
                    Period: "DAY",
                },
                Throttle: &ThrottleConfig{
                    BurstLimit: 10,
                    RateLimit:  5,
                },
            },

            // Optional: Add custom domain
            CustomDomain: &CustomDomainConfig{
                DomainName:     "api.example.com",
                CertificateArn: "arn:aws:acm:us-east-1:123456789012:certificate/abc123...",
            },

            // Add tags
            Tags: map[string]string{
                "Team":        "Platform",
                "Department":  "Engineering",
            },
            Environment: "production",
        })
        if err != nil {
            return err
        }

        // Export important values
        ctx.Export("apiUrl", api.BaseURL)
        ctx.Export("customDomainUrl", api.CustomDomainURL)
        
        // If you created an API key, you might want to export it
        if api.APIKey != nil {
            ctx.Export("apiKeyId", api.APIKey.ID())
        }

        return nil
    })
}
```