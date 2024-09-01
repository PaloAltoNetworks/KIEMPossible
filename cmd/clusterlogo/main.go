/*

- Flattened - the verbs that only work for specific resources needs to be documented
- Subresources level is where we flatten to
- Docs - write what it accounts for - stuff like resourceNames, all permissions are individually handled (some may appear twice for the same source if given by different granters). Permissions given specifically to subresources are shown as such in the db
- Docs - inclusterconfig as default, fallback to kubeconfig - kubeconfig handling separate for each CSP
- Docs - groups removed

- AWS - permission to get creds and logs
- Azure - local kubernetes accounts need to be enabled (for the fetching of the kubeconfig), permissions to get clusteradmin credfs and get logs
- GCP - currently stores all the flattened permission, doesn't get logs yet (60 request limit, need to incorporate something like pub sub, bigquery)


NICE-TO-HAVE
- Add check for stale resource? roles/clusterroles with no bindings? sa with no permissions
- AWS Users given access through either configmap or access entries & policies - need to check if can support
- Azure entities are given access based on UUID
- Improve DB speed when checking against eventLogs

URGENT
- Add support for namespaces as both namespaced and cluster-wide: binding_collection line 100 add if for resources that are both namespaced and cluster-wide, also add for crb loops

VALIDATION
- Validate permission-scope and last-used resource logic
- Check progress bars - added for local log handling and in azure and aws for log handling

- Talk name:
Ready? Set... ClusterLoGo! Exploring Least Privileged Through Kubernetes Logs
Golang with Golan - Exploring Kubernetes Permissions through Logs
*/

package main

func main() {
	Collect()
}
