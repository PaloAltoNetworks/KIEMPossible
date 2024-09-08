/* TODO
NICE-TO-HAVE
- Add check for stale resource? roles/clusterroles with no bindings? sa with no permissions
- AWS Users given access through access entries & policies - need to check if can support (EKS API rather than kube). Configmap is through groups so will be in calcd as part of log handling, same as access entries' group inheritance
- Add Azure AD UUID resolution at the end maybe?
- Add default queries


VALIDATION
- Check progress bars for local
- Check user permissions based on groups in log for local

- Talk name:
Ready? Set... ClusterLoGo! Exploring Least Privileged Through Kubernetes Logs
Golang with Golan - Exploring Kubernetes Permissions through Logs
*/

package main

func main() {
	Collect()
}
