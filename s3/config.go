package s3

import "time"

// Config is the connection: the S3 endpoint, the endpoint browsers reach, the region, the bucket and the keys.
type Config struct {
	Endpoint        string
	BrowserEndpoint string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	Timeout         time.Duration
}
