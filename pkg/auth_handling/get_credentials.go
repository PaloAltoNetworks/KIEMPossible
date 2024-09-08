package auth_handling

import (
	"flag"
	"fmt"
	"os"
)

type CredentialsPath struct {
	FilePath string
	LogFile  string
}

type ClusterInfo struct {
	ClusterName string
	WorkspaceID string
	Sub         string
	RG          string
	ProjectID   string
	Region      string
}

func AcceptCredentials(awsCredentialsFile, awsClusterName, azureCredentialsFile, azureClusterName, azureWorkspaceID, azureSubscriptionID, azureResourceGroup, gcpCredentialsFile, gcpClusterName, gcpProjectID, gcpRegion, logFile string) (CredentialsPath, ClusterInfo, error) {
	if logFile != "" {
		return CredentialsPath{LogFile: logFile}, ClusterInfo{}, nil
	}

	if awsCredentialsFile != "" {
		return CredentialsPath{FilePath: awsCredentialsFile}, ClusterInfo{ClusterName: awsClusterName}, nil
	}

	if azureCredentialsFile != "" {
		return CredentialsPath{FilePath: azureCredentialsFile}, ClusterInfo{ClusterName: azureClusterName, WorkspaceID: azureWorkspaceID, Sub: azureSubscriptionID, RG: azureResourceGroup}, nil
	}

	if gcpCredentialsFile != "" {
		return CredentialsPath{FilePath: gcpCredentialsFile}, ClusterInfo{ClusterName: gcpClusterName, ProjectID: gcpProjectID, Region: gcpRegion}, nil
	}

	return CredentialsPath{}, ClusterInfo{}, fmt.Errorf("no valid credentials provided")
}

func Authenticator() (CredentialsPath, ClusterInfo, string) {
	var cmd = flag.NewFlagSet("auth", flag.ExitOnError)
	var awsCmd = flag.NewFlagSet("aws", flag.ExitOnError)
	var azureCmd = flag.NewFlagSet("azure", flag.ExitOnError)
	var gcpCmd = flag.NewFlagSet("gcp", flag.ExitOnError)
	var localCmd = flag.NewFlagSet("local", flag.ExitOnError)

	// AWS flags
	awsCredentialsFile := awsCmd.String("credentials-file", "", "AWS credentials file path including the 'AWS_REGION=' variable")
	awsClusterName := awsCmd.String("cluster-name", "", "AWS cluster name")

	// Azure flags
	azureCredentialsFile := azureCmd.String("credentials-file", "", "Path to a File with SP Credentials, Structure is:\nAZURE_TENANT_ID=<tenant_id>\nAZURE_CLIENT_ID=<client_id>\nAZURE_CLIENT_SECRET=<secret>")
	azureClusterName := azureCmd.String("cluster-name", "", "Azure cluster name")
	azureWorkspaceID := azureCmd.String("workspace-id", "", "Azure log analytics workspace ID")
	azureSubscriptionID := azureCmd.String("sub-id", "", "Azure subscription ID")
	azureResourceGroup := azureCmd.String("rg", "", "Azure resource group")

	// GCP flags
	gcpCredentialsFile := gcpCmd.String("credentials-file", "", "GCP credentials file path to a service account JSON key file")
	gcpClusterName := gcpCmd.String("cluster-name", "", "GCP cluster name")
	gcpProjectID := gcpCmd.String("project-id", "", "GCP project id")
	gcpRegion := gcpCmd.String("region", "", "GCP region")

	// Local flags
	logFile := localCmd.String("log-file", "", "Path to log file")

	var args []string
	if len(os.Args) > 1 {
		args = os.Args[1:]
	}

	cmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [command] [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Available commands:\n")
		fmt.Fprintf(os.Stderr, "  aws\tUse for EKS Clusters\n")
		fmt.Fprintf(os.Stderr, "  azure\tUse for AKS Clusters\n")
		fmt.Fprintf(os.Stderr, "  gcp\tUse for GKE Clusters\n")
		fmt.Fprintf(os.Stderr, "  local\tUse local log file\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Use '%s [command] -help' for command-specific help.\n", os.Args[0])
	}

	if len(args) < 1 {
		cmd.Usage()
		os.Exit(1)
	}

	switch args[0] {
	case "aws":
		awsCmd.Parse(args[1:])
	case "azure":
		azureCmd.Parse(args[1:])
	case "gcp":
		gcpCmd.Parse(args[1:])
	case "local":
		localCmd.Parse(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		cmd.Usage()
		os.Exit(1)
	}

	var credentialsPath CredentialsPath
	var clusterInfo ClusterInfo
	var cloudProvider string
	var err error

	switch args[0] {
	case "aws":
		cloudProvider = "aws"
		credentialsPath, clusterInfo, err = AcceptCredentials(*awsCredentialsFile, *awsClusterName, "", "", "", "", "", "", "", "", "", "")
	case "azure":
		cloudProvider = "azure"
		credentialsPath, clusterInfo, err = AcceptCredentials("", "", *azureCredentialsFile, *azureClusterName, *azureWorkspaceID, *azureSubscriptionID, *azureResourceGroup, "", "", "", "", "")
	case "gcp":
		cloudProvider = "gcp"
		credentialsPath, clusterInfo, err = AcceptCredentials("", "", "", "", "", "", "", *gcpCredentialsFile, *gcpClusterName, *gcpProjectID, *gcpRegion, "")
	case "local":
		cloudProvider = "local"
		credentialsPath, clusterInfo, err = AcceptCredentials("", "", "", "", "", "", "", "", "", "", "", *logFile)
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
