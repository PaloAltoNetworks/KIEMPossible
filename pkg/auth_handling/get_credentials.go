package auth_handling

import (
	"flag"
	"fmt"
	"os"
)

// Establish command line flows and help for aws, azure, gcp and local. Returns the credential information

type CredentialsPath struct {
	FilePath         string
	LogFile          string
	TenantID         string
	ClientID         string
	ClientSecret     string
	CollectWorkloads bool
	ShouldAdvise     bool
}

type ClusterInfo struct {
	ClusterName string
	WorkspaceID string
	Sub         string
	RG          string
	ProjectID   string
	Region      string
}

func AcceptCredentials(awsClusterName, azureTenantID, azureClientID, azureClientSecret, azureClusterName, azureWorkspaceID, azureSubscriptionID, azureResourceGroup, gcpCredentialsFile, gcpClusterName, gcpProjectID, gcpRegion, logFile string, collectWorkloads bool) (CredentialsPath, ClusterInfo, error) {
	if logFile != "" {
		return CredentialsPath{LogFile: logFile, CollectWorkloads: collectWorkloads}, ClusterInfo{}, nil
	}

	if awsClusterName != "" {
		return CredentialsPath{CollectWorkloads: collectWorkloads}, ClusterInfo{ClusterName: awsClusterName}, nil
	}

	if azureTenantID != "" && azureClientID != "" && azureClientSecret != "" {
		return CredentialsPath{TenantID: azureTenantID, ClientID: azureClientID, ClientSecret: azureClientSecret, CollectWorkloads: collectWorkloads}, ClusterInfo{ClusterName: azureClusterName, WorkspaceID: azureWorkspaceID, Sub: azureSubscriptionID, RG: azureResourceGroup}, nil
	}

	if gcpCredentialsFile != "" {
		return CredentialsPath{FilePath: gcpCredentialsFile, CollectWorkloads: collectWorkloads}, ClusterInfo{ClusterName: gcpClusterName, ProjectID: gcpProjectID, Region: gcpRegion}, nil
	}

	return CredentialsPath{}, ClusterInfo{}, fmt.Errorf("no valid credentials provided")
}

func Authenticator() (CredentialsPath, ClusterInfo, string) {
	var cmd = flag.NewFlagSet("auth", flag.ExitOnError)
	var awsCmd = flag.NewFlagSet("aws", flag.ExitOnError)
	var azureCmd = flag.NewFlagSet("azure", flag.ExitOnError)
	var gcpCmd = flag.NewFlagSet("gcp", flag.ExitOnError)
	var localCmd = flag.NewFlagSet("local", flag.ExitOnError)

	// Add collect-workloads flag to all subcommands
	awsCollectWorkloads := awsCmd.Bool("collect-workloads", false, "[OPTIONAL] Collect workload information")
	azureCollectWorkloads := azureCmd.Bool("collect-workloads", false, "[OPTIONAL] Collect workload information")
	gcpCollectWorkloads := gcpCmd.Bool("collect-workloads", false, "[OPTIONAL] Collect workload information")
	localCollectWorkloads := localCmd.Bool("collect-workloads", false, "[OPTIONAL] Collect workload information")

	// Add advise flag to all subcommands
	awsAdvise := awsCmd.Bool("advise", false, "[OPTIONAL] Run analysis and provide recommendations")
	azureAdvise := azureCmd.Bool("advise", false, "[OPTIONAL] Run analysis and provide recommendations")
	gcpAdvise := gcpCmd.Bool("advise", false, "[OPTIONAL] Run analysis and provide recommendations")
	localAdvise := localCmd.Bool("advise", false, "[OPTIONAL] Run analysis and provide recommendations")

	awsClusterName := awsCmd.String("cluster-name", "", "AWS cluster name")

	azureClusterName := azureCmd.String("cluster-name", "", "Azure cluster name")
	azureTenantID := azureCmd.String("tenant-id", "", "Azure tenant ID")
	azureSubscriptionID := azureCmd.String("subscription", "", "Azure subscription ID")
	azureResourceGroup := azureCmd.String("resource-group", "", "Azure resource group")
	azureClientID := azureCmd.String("client-id", "", "Azure service principal client ID")
	azureClientSecret := azureCmd.String("client-secret", "", "Azure service principal client secret")
	azureWorkspaceID := azureCmd.String("workspace-id", "", "Azure log analytics workspace ID")

	gcpCredentialsFile := gcpCmd.String("credentials-file", "", "GCP credentials file path to a service account JSON key file")
	gcpClusterName := gcpCmd.String("cluster-name", "", "GCP cluster name")
	gcpProjectID := gcpCmd.String("project-id", "", "GCP project id")
	gcpRegion := gcpCmd.String("region", "", "GCP region")

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
		credentialsPath, clusterInfo, err = AcceptCredentials(*awsClusterName, "", "", "", "", "", "", "", "", "", "", "", "", *awsCollectWorkloads)
		credentialsPath.ShouldAdvise = *awsAdvise
	case "azure":
		cloudProvider = "azure"
		credentialsPath, clusterInfo, err = AcceptCredentials("", *azureTenantID, *azureClientID, *azureClientSecret, *azureClusterName, *azureWorkspaceID, *azureSubscriptionID, *azureResourceGroup, "", "", "", "", "", *azureCollectWorkloads)
		credentialsPath.ShouldAdvise = *azureAdvise
	case "gcp":
		cloudProvider = "gcp"
		credentialsPath, clusterInfo, err = AcceptCredentials("", "", "", "", "", "", "", "", *gcpCredentialsFile, *gcpClusterName, *gcpProjectID, *gcpRegion, "", *gcpCollectWorkloads)
		credentialsPath.ShouldAdvise = *gcpAdvise
	case "local":
		cloudProvider = "local"
		credentialsPath, clusterInfo, err = AcceptCredentials("", "", "", "", "", "", "", "", "", "", "", "", *logFile, *localCollectWorkloads)
		credentialsPath.ShouldAdvise = *localAdvise
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
