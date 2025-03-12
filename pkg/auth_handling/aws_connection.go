package auth_handling

import (
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

// Connect to AWS using creds file which contains all necessary info
func AwsAuth(credentialsPath CredentialsPath) (*session.Session, error) {
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")
	region := os.Getenv("AWS_REGION")

	// Set a default region if not provided
	if region == "" {
		region = "us-east-1"
	}

	awsConfig := &aws.Config{
		Credentials: credentials.NewStaticCredentials(accessKeyID, secretAccessKey, sessionToken),
		Region:      aws.String(region),
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, err
	}

	return sess, nil
}
