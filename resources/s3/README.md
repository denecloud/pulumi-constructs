Example

```
func main() {
    pulumi.Run(func(ctx *pulumi.Context) error {
        bucket, err := NewSecureBucket(ctx, "my-secure-bucket", &BucketConfig{
            BucketName:   "my-company-bucket",
            Environment:  "dev",
            Tags: map[string]string{
                "Team": "Platform",
            },
        })
        if err != nil {
            return err
        }

        // Export values
        ctx.Export("bucketName", bucket.Bucket.ID())
        ctx.Export("bucketArn", bucket.BucketArn)
        return nil
    })
}
```