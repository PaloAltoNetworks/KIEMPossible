package auth_handling

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"golang.org/x/oauth2/google"
)

type CredentialsPath struct {
	FilePath string
	LogFile  string
}

type ClusterInfo struct {
	ClusterName string
	WorkspaceID string
	ProjectID   string
	Region      string
}

func AcceptCredentials(awsCredentialsFile, awsClusterName, azureCredentialsFile, azureClusterName, azureWorkspaceID, gcpCredentialsFile, gcpClusterName, gcpProjectID, gcpRegion, logFile string) (CredentialsPath, ClusterInfo, error) {
	if logFile != "" {
		return CredentialsPath{LogFile: logFile}, ClusterInfo{}, nil
	}

	if awsCredentialsFile != "" {
		return CredentialsPath{FilePath: awsCredentialsFile}, ClusterInfo{ClusterName: awsClusterName}, nil
	}

	if azureCredentialsFile != "" {
		return CredentialsPath{FilePath: azureCredentialsFile}, ClusterInfo{ClusterName: azureClusterName, WorkspaceID: azureWorkspaceID}, nil
	}

	if gcpCredentialsFile != "" {
		return CredentialsPath{FilePath: gcpCredentialsFile}, ClusterInfo{ClusterName: gcpClusterName, ProjectID: gcpProjectID, Region: gcpRegion}, nil
	}

	return CredentialsPath{}, ClusterInfo{}, fmt.Errorf("no valid credentials provided")
}

var (
	awsChosen     bool
	azureChosen   bool
	gcpChosen     bool
	logChosen     bool
	cloudProvider string
)

func Authenticator() (CredentialsPath, ClusterInfo, string) {
	awsCredentialsFile := flag.String("aws-credentials-file", "", "AWS credentials file path including the 'AWS_REGION=' variable")
	awsClusterName := flag.String("aws-cluster-name", "", "AWS cluster name")

	azureCredentialsFile := flag.String("azure-credentials-file", "", "Path to a File with SP Credentials, Structure is:\nAZURE_TENANT_ID=<tenant_id>\nAZURE_CLIENT_ID=<client_id>\nAZURE_CLIENT_SECRET=<secret>")
	azureClusterName := flag.String("azure-cluster-name", "", "Azure cluster name")
	azureWorkspaceID := flag.String("azure-workspace-id", "", "Azure log analytics workspace ID")

	gcpCredentialsFile := flag.String("gcp-credentials-file", "", "GCP credentials file path to a service account JSON key file")
	gcpClusterName := flag.String("gcp-cluster-name", "", "GCP cluster name")
	gcpProjectID := flag.String("gcp-project-id", "", "GCP project id")
	gcpRegion := flag.String("gcp-region", "", "GCP region")

	logFile := flag.String("log-file", "", "Path to log file")
	help := flag.Bool("help", false, "Show help")

	flag.Parse()

	if *help {
		flag.PrintDefaults()
		os.Exit(0)
	}

	if *awsCredentialsFile != "" || *awsClusterName != "" {
		awsChosen = true
		cloudProvider = "aws"
	} else if *azureCredentialsFile != "" || *azureClusterName != "" {
		azureChosen = true
		cloudProvider = "azure"
	} else if *gcpCredentialsFile != "" || *gcpClusterName != "" {
		gcpChosen = true
		cloudProvider = "gcp"
	} else if *logFile != "" {
		logChosen = true
		cloudProvider = "log"
	}

	if awsChosen {
		if *azureCredentialsFile != "" || *azureClusterName != "" {
			fmt.Println("Error: Only one cloud provider (AWS, Azure, or GCP) can be used at a time")
			os.Exit(1)
		}
		if *gcpCredentialsFile != "" || *gcpClusterName != "" {
			fmt.Println("Error: Only one cloud provider (AWS, Azure, or GCP) can be used at a time")
			os.Exit(1)
		}
		if *logFile != "" {
			fmt.Println("Error: Attempt to specify log file and cloud provider")
			os.Exit(1)
		}
	} else if azureChosen {
		if *awsCredentialsFile != "" || *awsClusterName != "" {
			fmt.Println("Error: Only one cloud provider (AWS, Azure, or GCP) can be used at a time")
			os.Exit(1)
		}
		if *gcpCredentialsFile != "" || *gcpClusterName != "" {
			fmt.Println("Error: Only one cloud provider (AWS, Azure, or GCP) can be used at a time")
			os.Exit(1)
		}
		if *logFile != "" {
			fmt.Println("Error: Attempt to specify log file and cloud provider")
			os.Exit(1)
		}
	} else if gcpChosen {
		if *awsCredentialsFile != "" || *awsClusterName != "" {
			fmt.Println("Error: Only one cloud provider (AWS, Azure, or GCP) can be used at a time")
			os.Exit(1)
		}
		if *azureCredentialsFile != "" || *azureClusterName != "" {
			fmt.Println("Error: Only one cloud provider (AWS, Azure, or GCP) can be used at a time")
			os.Exit(1)
		}
		if *logFile != "" {
			fmt.Println("Error: Attempt to specify log file and cloud provider")
			os.Exit(1)
		}
	} else if logChosen {
		if *awsCredentialsFile != "" || *awsClusterName != "" {
			fmt.Println("Error: Attempt to specify log file and cloud provider")
			os.Exit(1)
		}
		if *azureCredentialsFile != "" || *azureClusterName != "" {
			fmt.Println("Error: Attempt to specify log file and cloud provider")
			os.Exit(1)
		}
		if *gcpCredentialsFile != "" || *gcpClusterName != "" {
			fmt.Println("Error: Attempt to specify log file and cloud provider")
			os.Exit(1)
		}
	} else {
		fmt.Println("Error: No cloud provider specified")
		os.Exit(1)
	}

	var credentialsPath CredentialsPath
	var clusterInfo ClusterInfo
	var err error

	switch cloudProvider {
	case "aws":
		credentialsPath, clusterInfo, err = AcceptCredentials(*awsCredentialsFile, *awsClusterName, "", "", "", "", "", "", "", "")
	case "azure":
		credentialsPath, clusterInfo, err = AcceptCredentials("", "", *azureCredentialsFile, *azureClusterName, *azureWorkspaceID, "", "", "", "", "")
	case "gcp":
		credentialsPath, clusterInfo, err = AcceptCredentials("", "", "", "", "", *gcpCredentialsFile, *gcpClusterName, *gcpProjectID, *gcpRegion, "")
	case "log":
		credentialsPath, clusterInfo, err = AcceptCredentials("", "", "", "", "", "", "", "", "", *logFile)
	default:
		fmt.Println("Error: Invalid cloud provider")
		os.Exit(1)
	}

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return credentialsPath, clusterInfo, cloudProvider
}

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

func AzureAuth(credentialsPath CredentialsPath) (*azidentity.ClientSecretCredential, error) {
	filePath := credentialsPath.FilePath

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var tenantID, clientID, clientSecret string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := strings.TrimSpace(parts[1])
			switch key {
			case "AZURE_TENANT_ID":
				tenantID = value
			case "AZURE_CLIENT_ID":
				clientID = value
			case "AZURE_CLIENT_SECRET":
				clientSecret = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, err
	}

	return cred, nil
}

func GCPAuth(credentialsPath CredentialsPath) (*google.Credentials, error) {
	filePath := credentialsPath.FilePath

	jsonKey, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service account key file: %v", err)
	}
	creds, err := google.CredentialsFromJSON(context.Background(), jsonKey, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to create Google Credentials: %v", err)
	}
	return creds, nil
}
