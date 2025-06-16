package kube_collection

import (
	"context"
	"database/sql"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type WorkloadInfo struct {
	WorkloadType       string
	WorkloadName       string
	ServiceAccountName string
	OriginalOwnerType  string
	OriginalOwnerName  string
}

func getOwnerInfo(obj metav1.Object, resourceType string) (string, string) {
	if len(obj.GetOwnerReferences()) > 0 {
		return obj.GetOwnerReferences()[0].Kind, obj.GetOwnerReferences()[0].Name
	}
	return resourceType, obj.GetName()
}

func Collect_workloads(client *kubernetes.Clientset, db *sql.DB) error {
	// Get all namespaces
	namespaces, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Prepare the insert statement
	stmt, err := db.Prepare(`
		INSERT INTO rufus.workload_identities (
			workload_type, workload_name, 
			service_account_name, original_owner_type, original_owner_name
		) VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			service_account_name = VALUES(service_account_name),
			original_owner_type = VALUES(original_owner_type),
			original_owner_name = VALUES(original_owner_name)
	`)
	if err != nil {
		return fmt.Errorf("error preparing statement: %v", err)
	}
	defer stmt.Close()

	// Collect workloads info by namespace
	for _, namespace := range namespaces.Items {
		pods, err := collect_pods(client, namespace)
		if err != nil {
			return err
		}
		for _, pod := range pods {
			_, err = stmt.Exec(
				pod.WorkloadType,
				pod.WorkloadName,
				pod.ServiceAccountName,
				pod.OriginalOwnerType,
				pod.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting pod workload: %v", err)
			}
		}

		deployments, err := collect_deployments(client, namespace)
		if err != nil {
			return err
		}
		for _, deployment := range deployments {
			_, err = stmt.Exec(
				deployment.WorkloadType,
				deployment.WorkloadName,
				deployment.ServiceAccountName,
				deployment.OriginalOwnerType,
				deployment.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting deployment workload: %v", err)
			}
		}

		daemonsets, err := collect_daemonsets(client, namespace)
		if err != nil {
			return err
		}
		for _, daemonset := range daemonsets {
			_, err = stmt.Exec(
				daemonset.WorkloadType,
				daemonset.WorkloadName,
				daemonset.ServiceAccountName,
				daemonset.OriginalOwnerType,
				daemonset.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting daemonset workload: %v", err)
			}
		}

		replicasets, err := collect_replicasets(client, namespace)
		if err != nil {
			return err
		}
		for _, replicaset := range replicasets {
			_, err = stmt.Exec(
				replicaset.WorkloadType,
				replicaset.WorkloadName,
				replicaset.ServiceAccountName,
				replicaset.OriginalOwnerType,
				replicaset.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting replicaset workload: %v", err)
			}
		}

		statefulsets, err := collect_statefulsets(client, namespace)
		if err != nil {
			return err
		}
		for _, statefulset := range statefulsets {
			_, err = stmt.Exec(
				statefulset.WorkloadType,
				statefulset.WorkloadName,
				statefulset.ServiceAccountName,
				statefulset.OriginalOwnerType,
				statefulset.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting statefulset workload: %v", err)
			}
		}

		jobs, err := collect_jobs(client, namespace)
		if err != nil {
			return err
		}
		for _, job := range jobs {
			_, err = stmt.Exec(
				job.WorkloadType,
				job.WorkloadName,
				job.ServiceAccountName,
				job.OriginalOwnerType,
				job.OriginalOwnerName,
			)
			if err != nil {
				return fmt.Errorf("error inserting job workload: %v", err)
			}
		}

		cronjobs, err := collect_cronjobs(client, namespace)
		if err != nil {
			return err
		}
		for _, cronjob := range cronjobs {
			_, err = stmt.Exec(
				cronjob.WorkloadType,
				cronjob.WorkloadName,
				cronjob.ServiceAccountName,
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

func collect_pods(client *kubernetes.Clientset, namespace corev1.Namespace) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	pods, err := client.CoreV1().Pods(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		if pod.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&pod, "pod")
			workload := WorkloadInfo{
				WorkloadType:       "Pod",
				WorkloadName:       pod.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, pod.Spec.ServiceAccountName),
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_deployments(client *kubernetes.Clientset, namespace corev1.Namespace) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	deployments, err := client.AppsV1().Deployments(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, deployment := range deployments.Items {
		if deployment.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&deployment, "deployment")
			workload := WorkloadInfo{
				WorkloadType:       "Deployment",
				WorkloadName:       deployment.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, deployment.Spec.Template.Spec.ServiceAccountName),
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_daemonsets(client *kubernetes.Clientset, namespace corev1.Namespace) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	daemonsets, err := client.AppsV1().DaemonSets(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, daemonset := range daemonsets.Items {
		if daemonset.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&daemonset, "daemonset")
			workload := WorkloadInfo{
				WorkloadType:       "DaemonSet",
				WorkloadName:       daemonset.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, daemonset.Spec.Template.Spec.ServiceAccountName),
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_replicasets(client *kubernetes.Clientset, namespace corev1.Namespace) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	replicasets, err := client.AppsV1().ReplicaSets(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, replicaset := range replicasets.Items {
		if replicaset.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&replicaset, "replicaset")
			workload := WorkloadInfo{
				WorkloadType:       "ReplicaSet",
				WorkloadName:       replicaset.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, replicaset.Spec.Template.Spec.ServiceAccountName),
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_statefulsets(client *kubernetes.Clientset, namespace corev1.Namespace) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	statefulsets, err := client.AppsV1().StatefulSets(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, statefulset := range statefulsets.Items {
		if statefulset.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&statefulset, "statefulset")
			workload := WorkloadInfo{
				WorkloadType:       "StatefulSet",
				WorkloadName:       statefulset.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, statefulset.Spec.Template.Spec.ServiceAccountName),
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_jobs(client *kubernetes.Clientset, namespace corev1.Namespace) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	jobs, err := client.BatchV1().Jobs(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, job := range jobs.Items {
		if job.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&job, "job")
			workload := WorkloadInfo{
				WorkloadType:       "Job",
				WorkloadName:       job.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, job.Spec.Template.Spec.ServiceAccountName),
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func collect_cronjobs(client *kubernetes.Clientset, namespace corev1.Namespace) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	cronjobs, err := client.BatchV1().CronJobs(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, cronjob := range cronjobs.Items {
		if cronjob.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName != "" {
			ownerType, ownerName := getOwnerInfo(&cronjob, "cronjob")
			workload := WorkloadInfo{
				WorkloadType:       "CronJob",
				WorkloadName:       cronjob.Name,
				ServiceAccountName: fmt.Sprintf("%s:%s", namespace.Name, cronjob.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName),
				OriginalOwnerType:  ownerType,
				OriginalOwnerName:  ownerName,
			}
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}
