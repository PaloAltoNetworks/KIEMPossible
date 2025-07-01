package kube_collection

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type WorkloadInfo struct {
	WorkloadType       string
	WorkloadName       string
	ServiceAccountName string
	WorkloadIdentity   string
	OriginalOwnerType  string
	OriginalOwnerName  string
}

func getOwnerInfo(obj metav1.Object, resourceType string) (string, string) {
	if len(obj.GetOwnerReferences()) > 0 {
		return obj.GetOwnerReferences()[0].Kind, obj.GetOwnerReferences()[0].Name
	}
	return resourceType, obj.GetName()
}

func Collect_workloads(client *kubernetes.Clientset, db *sql.DB, clusterType string, clusterName string, sess *session.Session) error {
	var eksPodIdentityMap map[string]string
	if clusterType == "EKS" && sess != nil {
		eksPodIdentityMap = make(map[string]string)
		eksSvc := eks.New(sess)
		// List pod identity associations
		listInput := &eks.ListPodIdentityAssociationsInput{
			ClusterName: &clusterName,
		}
		listOutput, err := eksSvc.ListPodIdentityAssociations(listInput)
		if err == nil {
			for _, assoc := range listOutput.Associations {
				if assoc.AssociationId != nil {
					descInput := &eks.DescribePodIdentityAssociationInput{
						ClusterName:   &clusterName,
						AssociationId: assoc.AssociationId,
					}
					descOutput, err := eksSvc.DescribePodIdentityAssociation(descInput)
					if err == nil && descOutput.Association != nil {
						ns := *descOutput.Association.Namespace
						sa := *descOutput.Association.ServiceAccount
						roleArn := *descOutput.Association.RoleArn
						key := ns + "/" + sa
						eksPodIdentityMap[key] = roleArn
					}
				}
			}
		}
	}

	// Get all namespaces
	namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Prepare the insert statement
	stmt, err := db.Prepare(`
		INSERT INTO rufus.workload_identities (
			workload_type, workload_name, 
			service_account_name, workload_identity, original_owner_type, original_owner_name
		) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			service_account_name = VALUES(service_account_name),
			workload_identity = VALUES(workload_identity),
			original_owner_type = VALUES(original_owner_type),
			original_owner_name = VALUES(original_owner_name)
	`)
	if err != nil {
		return fmt.Errorf("error preparing statement: %v", err)
	}
	defer stmt.Close()

	// Collect workloads info by namespace
	for _, namespace := range namespaces.Items {
		pods, err := collect_pods(client, namespace, clusterType, eksPodIdentityMap)
		if err != nil {
			return err
		}
		for _, pod := range pods {
			_, err = stmt.Exec(
				pod.WorkloadType,
				pod.WorkloadName,
				pod.ServiceAccountName,
				pod.WorkloadIdentity,
				pod.OriginalOwnerType,
				pod.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting pod workload: %v", err)
			}
		}

		deployments, err := collect_deployments(client, namespace, clusterType)
		if err != nil {
			return err
		}
		for _, deployment := range deployments {
			_, err = stmt.Exec(
				deployment.WorkloadType,
				deployment.WorkloadName,
				deployment.ServiceAccountName,
				deployment.WorkloadIdentity,
				deployment.OriginalOwnerType,
				deployment.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting deployment workload: %v", err)
			}
		}

		daemonsets, err := collect_daemonsets(client, namespace, clusterType)
		if err != nil {
			return err
		}
		for _, daemonset := range daemonsets {
			_, err = stmt.Exec(
				daemonset.WorkloadType,
				daemonset.WorkloadName,
				daemonset.ServiceAccountName,
				daemonset.WorkloadIdentity,
				daemonset.OriginalOwnerType,
				daemonset.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting daemonset workload: %v", err)
			}
		}

		replicasets, err := collect_replicasets(client, namespace, clusterType)
		if err != nil {
			return err
		}
		for _, replicaset := range replicasets {
			_, err = stmt.Exec(
				replicaset.WorkloadType,
				replicaset.WorkloadName,
				replicaset.ServiceAccountName,
				replicaset.WorkloadIdentity,
				replicaset.OriginalOwnerType,
				replicaset.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting replicaset workload: %v", err)
			}
		}

		statefulsets, err := collect_statefulsets(client, namespace, clusterType)
		if err != nil {
			return err
		}
		for _, statefulset := range statefulsets {
			_, err = stmt.Exec(
				statefulset.WorkloadType,
				statefulset.WorkloadName,
				statefulset.ServiceAccountName,
				statefulset.WorkloadIdentity,
				statefulset.OriginalOwnerType,
				statefulset.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting statefulset workload: %v", err)
			}
		}

		jobs, err := collect_jobs(client, namespace, clusterType)
		if err != nil {
			return err
		}
		for _, job := range jobs {
			_, err = stmt.Exec(
				job.WorkloadType,
				job.WorkloadName,
				job.ServiceAccountName,
				job.WorkloadIdentity,
				job.OriginalOwnerType,
				job.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting job workload: %v", err)
			}
		}

		cronjobs, err := collect_cronjobs(client, namespace, clusterType)
		if err != nil {
			return err
		}
		for _, cronjob := range cronjobs {
			_, err = stmt.Exec(
				cronjob.WorkloadType,
				cronjob.WorkloadName,
				cronjob.ServiceAccountName,
				cronjob.WorkloadIdentity,
				cronjob.OriginalOwnerType,
				cronjob.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting cronjob workload: %v", err)
			}
		}
	}

	return nil
}

func collect_pods(client *kubernetes.Clientset, namespace corev1.Namespace, clusterType string, eksPodIdentityMap map[string]string) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	pods, err := client.CoreV1().Pods(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		if pod.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&pod, "pod")
			workloadIdentity := ""
			// EKS pod identity logic
			if clusterType == "EKS" && eksPodIdentityMap != nil {
				for _, vol := range pod.Spec.Volumes {
					if vol.Name == "eks-pod-identity-token" {
						key := namespace.Name + "/" + pod.Spec.ServiceAccountName
						if roleArn, ok := eksPodIdentityMap[key]; ok {
							workloadIdentity = roleArn
						}
						break
					}
				}
			}
			// AKS logic
			if clusterType == "AKS" {
				labels := pod.Labels
				if val, ok := labels["azure.workload.identity/use"]; ok && val == "true" {
					// Check for AZURE_CLIENT_ID in env
					found := false
					for _, container := range pod.Spec.Containers {
						for _, env := range container.Env {
							if env.Name == "AZURE_CLIENT_ID" && env.Value != "" {
								workloadIdentity = env.Value
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						saName := pod.Spec.ServiceAccountName
						sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
						if err == nil {
							if val, ok := sa.Annotations["azure.workload.identity/client-id"]; ok {
								workloadIdentity = val
							}
						}
					}
				}
			}
			// GKE logic
			if clusterType == "GKE" {
				saName := pod.Spec.ServiceAccountName
				sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
				if err == nil {
					if val, ok := sa.Annotations["iam.gke.io/gcp-service-account"]; ok {
						workloadIdentity = val
					}
				}
			}
			workload := WorkloadInfo{
				WorkloadType:       "Pod",
				WorkloadName:       pod.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, pod.Spec.ServiceAccountName),
				WorkloadIdentity:   workloadIdentity,
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_deployments(client *kubernetes.Clientset, namespace corev1.Namespace, clusterType string) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	deployments, err := client.AppsV1().Deployments(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, deployment := range deployments.Items {
		if deployment.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&deployment, "deployment")
			workloadIdentity := ""
			// AKS logic
			if clusterType == "AKS" {
				labels := deployment.Spec.Template.Labels
				if val, ok := labels["azure.workload.identity/use"]; ok && val == "true" {
					// Check for AZURE_CLIENT_ID in env
					found := false
					for _, container := range deployment.Spec.Template.Spec.Containers {
						for _, env := range container.Env {
							if env.Name == "AZURE_CLIENT_ID" && env.Value != "" {
								workloadIdentity = env.Value
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						// Get ServiceAccount annotation
						saName := deployment.Spec.Template.Spec.ServiceAccountName
						sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
						if err == nil {
							if val, ok := sa.Annotations["azure.workload.identity/client-id"]; ok {
								workloadIdentity = val
							}
						}
					}
				}
			}
			// GKE logic
			if clusterType == "GKE" {
				saName := deployment.Spec.Template.Spec.ServiceAccountName
				sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
				if err == nil {
					if val, ok := sa.Annotations["iam.gke.io/gcp-service-account"]; ok {
						workloadIdentity = val
					}
				}
			}
			workload := WorkloadInfo{
				WorkloadType:       "Deployment",
				WorkloadName:       deployment.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, deployment.Spec.Template.Spec.ServiceAccountName),
				WorkloadIdentity:   workloadIdentity,
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_daemonsets(client *kubernetes.Clientset, namespace corev1.Namespace, clusterType string) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	daemonsets, err := client.AppsV1().DaemonSets(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, daemonset := range daemonsets.Items {
		if daemonset.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&daemonset, "daemonset")
			workloadIdentity := ""
			// AKS logic
			if clusterType == "AKS" {
				labels := daemonset.Spec.Template.Labels
				if val, ok := labels["azure.workload.identity/use"]; ok && val == "true" {
					found := false
					for _, container := range daemonset.Spec.Template.Spec.Containers {
						for _, env := range container.Env {
							if env.Name == "AZURE_CLIENT_ID" && env.Value != "" {
								workloadIdentity = env.Value
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						saName := daemonset.Spec.Template.Spec.ServiceAccountName
						sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
						if err == nil {
							if val, ok := sa.Annotations["azure.workload.identity/client-id"]; ok {
								workloadIdentity = val
							}
						}
					}
				}
			}
			// GKE logic
			if clusterType == "GKE" {
				saName := daemonset.Spec.Template.Spec.ServiceAccountName
				sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
				if err == nil {
					if val, ok := sa.Annotations["iam.gke.io/gcp-service-account"]; ok {
						workloadIdentity = val
					}
				}
			}
			workload := WorkloadInfo{
				WorkloadType:       "DaemonSet",
				WorkloadName:       daemonset.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, daemonset.Spec.Template.Spec.ServiceAccountName),
				WorkloadIdentity:   workloadIdentity,
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_replicasets(client *kubernetes.Clientset, namespace corev1.Namespace, clusterType string) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	replicasets, err := client.AppsV1().ReplicaSets(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, replicaset := range replicasets.Items {
		if replicaset.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&replicaset, "replicaset")
			workloadIdentity := ""
			// AKS logic
			if clusterType == "AKS" {
				labels := replicaset.Spec.Template.Labels
				if val, ok := labels["azure.workload.identity/use"]; ok && val == "true" {
					found := false
					for _, container := range replicaset.Spec.Template.Spec.Containers {
						for _, env := range container.Env {
							if env.Name == "AZURE_CLIENT_ID" && env.Value != "" {
								workloadIdentity = env.Value
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						saName := replicaset.Spec.Template.Spec.ServiceAccountName
						sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
						if err == nil {
							if val, ok := sa.Annotations["azure.workload.identity/client-id"]; ok {
								workloadIdentity = val
							}
						}
					}
				}
			}
			// GKE logic
			if clusterType == "GKE" {
				saName := replicaset.Spec.Template.Spec.ServiceAccountName
				sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
				if err == nil {
					if val, ok := sa.Annotations["iam.gke.io/gcp-service-account"]; ok {
						workloadIdentity = val
					}
				}
			}
			workload := WorkloadInfo{
				WorkloadType:       "ReplicaSet",
				WorkloadName:       replicaset.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, replicaset.Spec.Template.Spec.ServiceAccountName),
				WorkloadIdentity:   workloadIdentity,
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_statefulsets(client *kubernetes.Clientset, namespace corev1.Namespace, clusterType string) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	statefulsets, err := client.AppsV1().StatefulSets(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, statefulset := range statefulsets.Items {
		if statefulset.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&statefulset, "statefulset")
			workloadIdentity := ""
			// AKS logic
			if clusterType == "AKS" {
				labels := statefulset.Spec.Template.Labels
				if val, ok := labels["azure.workload.identity/use"]; ok && val == "true" {
					found := false
					for _, container := range statefulset.Spec.Template.Spec.Containers {
						for _, env := range container.Env {
							if env.Name == "AZURE_CLIENT_ID" && env.Value != "" {
								workloadIdentity = env.Value
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						saName := statefulset.Spec.Template.Spec.ServiceAccountName
						sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
						if err == nil {
							if val, ok := sa.Annotations["azure.workload.identity/client-id"]; ok {
								workloadIdentity = val
							}
						}
					}
				}
			}
			// GKE logic
			if clusterType == "GKE" {
				saName := statefulset.Spec.Template.Spec.ServiceAccountName
				sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
				if err == nil {
					if val, ok := sa.Annotations["iam.gke.io/gcp-service-account"]; ok {
						workloadIdentity = val
					}
				}
			}
			workload := WorkloadInfo{
				WorkloadType:       "StatefulSet",
				WorkloadName:       statefulset.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, statefulset.Spec.Template.Spec.ServiceAccountName),
				WorkloadIdentity:   workloadIdentity,
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_jobs(client *kubernetes.Clientset, namespace corev1.Namespace, clusterType string) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	jobs, err := client.BatchV1().Jobs(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, job := range jobs.Items {
		if job.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&job, "job")
			workloadIdentity := ""
			// AKS logic
			if clusterType == "AKS" {
				labels := job.Spec.Template.Labels
				if val, ok := labels["azure.workload.identity/use"]; ok && val == "true" {
					found := false
					for _, container := range job.Spec.Template.Spec.Containers {
						for _, env := range container.Env {
							if env.Name == "AZURE_CLIENT_ID" && env.Value != "" {
								workloadIdentity = env.Value
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						saName := job.Spec.Template.Spec.ServiceAccountName
						sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
						if err == nil {
							if val, ok := sa.Annotations["azure.workload.identity/client-id"]; ok {
								workloadIdentity = val
							}
						}
					}
				}
			}
			// GKE logic
			if clusterType == "GKE" {
				saName := job.Spec.Template.Spec.ServiceAccountName
				sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
				if err == nil {
					if val, ok := sa.Annotations["iam.gke.io/gcp-service-account"]; ok {
						workloadIdentity = val
					}
				}
			}
			workload := WorkloadInfo{
				WorkloadType:       "Job",
				WorkloadName:       job.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, job.Spec.Template.Spec.ServiceAccountName),
				WorkloadIdentity:   workloadIdentity,
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_cronjobs(client *kubernetes.Clientset, namespace corev1.Namespace, clusterType string) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	cronjobs, err := client.BatchV1().CronJobs(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, cronjob := range cronjobs.Items {
		if cronjob.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&cronjob, "cronjob")
			workloadIdentity := ""
			// AKS logic
			if clusterType == "AKS" {
				labels := cronjob.Spec.JobTemplate.Spec.Template.Labels
				if val, ok := labels["azure.workload.identity/use"]; ok && val == "true" {
					found := false
					for _, container := range cronjob.Spec.JobTemplate.Spec.Template.Spec.Containers {
						for _, env := range container.Env {
							if env.Name == "AZURE_CLIENT_ID" && env.Value != "" {
								workloadIdentity = env.Value
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						saName := cronjob.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName
						sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
						if err == nil {
							if val, ok := sa.Annotations["azure.workload.identity/client-id"]; ok {
								workloadIdentity = val
							}
						}
					}
				}
			}
			// GKE logic
			if clusterType == "GKE" {
				saName := cronjob.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName
				sa, err := client.CoreV1().ServiceAccounts(namespace.Name).Get(context.TODO(), saName, metav1.GetOptions{})
				if err == nil {
					if val, ok := sa.Annotations["iam.gke.io/gcp-service-account"]; ok {
						workloadIdentity = val
					}
				}
			}
			workload := WorkloadInfo{
				WorkloadType:       "CronJob",
				WorkloadName:       cronjob.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, cronjob.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName),
				WorkloadIdentity:   workloadIdentity,
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}
