package auth_handling

import (
	"bufio"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

func AwsAuth(credentialsPath CredentialsPath) (*session.Session, error) {
	filePath := credentialsPath.FilePath

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var accessKeyID, secretAccessKey, sessionToken string
	region := "us-east-1"

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := strings.TrimSpace(parts[1])
			switch key {
			case "AWS_ACCESS_KEY_ID":
				accessKeyID = value
			case "AWS_SECRET_ACCESS_KEY":
				secretAccessKey = value
			case "AWS_SESSION_TOKEN":
				sessionToken = value
			case "AWS_REGION":
				region = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
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
