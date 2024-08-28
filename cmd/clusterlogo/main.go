/*

- Flattened - the verbs that only work for specific resources needs to be documented
- Docs - write what it accounts for - stuff like resourceNames, all permissions are individually handled (some may appear twice for the same source if given by different granters). Permissions given specifically to subresources are shown as such in the db
- Docs - inclusterconfig as default, fallback to kubeconfig - kubeconfig handling separate for each CSP
- Docs - groups removed

- AWS - permission to get creds and logs
- Azure - local kubernetes accounts need to be enabled (for the fetching of the kubeconfig), permissions to get clusteradmin credfs and get logs


- Add "Local Logic", gcp logic
- Add check for stale resource? roles/clusterroles with no bindings?
- AWS Users given access through either configmap or access entries & policies - need to check if can support
- Check AWS logs for subresource (are they in a separate field or in the resource field?)
- Add "flattening" to subresource in initial cluster fetching
- Add support for namespaces as both namespaced and cluster-wide
- Improve DB speed when checking against eventLogs
- Check permission-scope and last-used resource logic



- Talk name:
Ready? Set... ClusterLoGo! Exploring Least Privileged Through Kubernetes Logs

*/

package main

func main() {
	Collect()
}

/* Self-Managed
Can only currently be run on the api server node (kind of)
Check the --audit-policy-file and --audit-log-path from the api server manifest
--audit-policy-file for log events and levels, --audit-log-path for the log file location


if webhook configured to ingest logs:
Check --audit-webhook-config-file for the webhook configuration (basically specialized kubeconfig)

potentially use last-applied for write permissions? -- https://kubernetes.io/docs/reference/kubectl/generated/kubectl_apply/kubectl_apply_view-last-applied/

Log structure:
https://kubernetes.io/docs/reference/config-api/apiserver-audit.v1/#audit-k8s-io-v1-Event
https://github.com/falcosecurity/plugins/tree/main/plugins/k8saudit
*/

/* GKE
GKE Audit logs - stored in a dedicated datastore in cloud logging
https://cloud.google.com/kubernetes-engine/docs/how-to/audit-logging
https://cloud.google.com/kubernetes-engine/docs/how-to/view-logs


https://cloud.google.com/logging/docs/reference/v2/rest/v2/entries/list
https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry


Query example
log_id("cloudaudit.googleapis.com/activity")
resource.type="k8s_cluster"
resource.labels.cluster_name="CLUSTER_NAME"
resource.labels.project_id="PROJECT_ID"
resource.labels.location="REGION"
protoPayload.request.metadata.name="WORKLOAD_NAME"

*/
