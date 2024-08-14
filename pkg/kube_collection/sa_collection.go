package kube_collection

import (
	"context"
	"database/sql"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func CollectServiceAccounts(client *kubernetes.Clientset, db *sql.DB) error {
	nsList, err := client.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Prepare the SQL statement to insert service accounts
	stmt, err := db.Prepare("INSERT INTO permission (entity_name) VALUES (?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ns := range nsList.Items {
		nsName := ns.Name

		// Get service accounts for the current namespace
		saList, err := client.CoreV1().ServiceAccounts(nsName).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		// Iterate through service accounts and insert into the database
		for _, sa := range saList.Items {
			saName := sa.Name
			key := fmt.Sprintf("%s:%s", nsName, saName)

			// Insert the service account into the database
			_, err = stmt.Exec(key)
			if err != nil {
				return err
			}

			fmt.Printf("Inserted service account %s into the database\n", key)
		}
	}

	return nil
}
