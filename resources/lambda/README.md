Example

```
func main() {
    pulumi.Run(func(ctx *pulumi.Context) error {
        // Create a Lambda function
        lambda, err := NewLambdaFunction(ctx, "my-function", &LambdaConfig{
            Runtime:     "nodejs18.x",
            Handler:     "index.handler",
            Code:        pulumi.NewFileArchive("./function.zip"),
            Description: "My awesome Lambda function",
            Environment: map[string]string{
                "STAGE": "production",
            },
            MemorySize: 256,
            Timeout:    30,
            EnableXRay: true,
            AlertConfig: &AlertConfig{
                ErrorThreshold:     1,
                DurationThreshold: 1000,
                NotificationARN:   "arn:aws:sns:us-east-1:123456789012:alerts",
            },
            Environment: "production",
            Tags: map[string]string{
                "Team": "Platform",
            },
        })
        if err != nil {
            return err
        }

        // Export values
        ctx.Export("functionName", lambda.FunctionName)
        ctx.Export("functionArn", lambda.FunctionArn)
        return nil
    })
}
```