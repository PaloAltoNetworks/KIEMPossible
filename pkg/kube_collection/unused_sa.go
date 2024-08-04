package kube_collection

func FindUnusedServiceAccounts(
	serviceAccounts *map[string]interface{},
	podAttachedSAs *map[string]string,
	issues *map[string]string,
) {
	for key := range *serviceAccounts {
		if _, ok := (*podAttachedSAs)[key]; !ok {
			issueKey := "[HYGIENE] Service Account Unused by Cluster Resources (ns-sa_name)"
			issueValue := key
			(*issues)[issueKey] = issueValue
		}
	}
}

// Check about SA usage in other ways - logs etc - could be used with token by 3rd party
