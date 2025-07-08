# KIEMPossible

<p align="center">
  <img src="./rufus.png" width="400" />
</p>

KIEMPossible is a tool designed to simplify Kubernetes Infrastructure Entitlement Management by allowing visibility of permissions and their usage across the cluster, to allow for real enforcement of the principle of least privilege (don't trust Rufus, he's a mole)

## Setup and Run
- `docker-compose up -d` - Spins up a mysql server on a Docker container, accessible at 127.0.0.1:3306 (`mysql -u mysql -p -h 127.0.0.1`, default password is 'mysql'). There is also an optional UI (uncomment in `docker-compose.yml`)
- `make darwin` - Creates a MacOS (amd64) executable in the /bin folder (KIEMPossible_darwin)
- `make linux` - Creates a Linux (amd64) executable in the /bin folder (KIEMPossible)
- `KIEMPossible_darwin [command] [options]` - Run MacOS version, command is the provider name
- `KIEMPossible [command] [options]` - Run Linux version, command is the provider name
- `--help or [command] --help` - Help menu for the binary and the individual commands 
- The concurrency limits for AWS and Azure are dynamic (based on CPU), and static for GCP (due to rate limit) - these can be changed by setting the `KIEMPOSSIBLE_LOG_CONCURRENCY` environment variable
- Log ingestion is set by default to look back 7 days - this can be changed by setting the `KIEMPOSSIBLE_LOG_DAYS` environment variable
- GCP page size for API requesets to Logging API is set at 1,000,000 by default - this can be changed by setting the `KIEMPOSSIBLE_GCP_PAGE_SIZE` environment variable
- Once ingestion and processing are finished, the tool will output a list of entities with unused dangerous permissions and workloads with dangerous permissions
- DISCLAIMER: when ingesting the logs, they are written to a temporary file, and removed once the tool is finished running. Depending on the amount of logs, this may take up substantial space on disk for the duration of the tool run

## Requirements
#### AWS
- Name of the target cluster
- Environment variables containing credentials (`AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN`. The region will be set to `us-east-1` by default unless `AWS_REGION` variable is set). It is recommended to set the session duration to 12 hours as reauthentication requires you to manually enter new credentials
- Permissions to get EKS credentials (within the cluster permissions to get Roles, ClusterRoles, RoleBindings, ClusterRoleBindings and Namespaces are required) 
- Audit logging configured for the cluster (`EKS->Cluster->Observability->Manage Logging->Audit`) and permissions to retrieve the logs 
- For the collect_workloads feature (optional), permissions to retrieve workloads within the cluster are required, and permissions to list and describe pod identity associations within AWS are required

#### AZURE
- Name of the target cluster
- Valid Service Principal credentials (`client-id, client-secret`) with permissions to query the log analytics workspace and permissions to get AKS user credentials (within the cluster permissions to get Roles, ClusterRoles, RoleBindings, ClusterRoleBindings and Namespaces are required)
- Name of the Resource Group in which the cluster is deployed
- Subscription ID of the Subscription in which the cluster is deployed
- Tenant ID of the tenant to which the subscription belongs
- Workspace ID of the Log Analytics Workspace which acts as the audit logs destination
- Audit logging configured for the cluster (`AKS->Cluster->Monitoring->Diagnostic Settings->Kubernetes Audit`) and permissions to retrieve the logs
- For the collect_workloads feature (optional), permissions to retrieve workloads within the cluster are required

#### GCP
- Name of the target cluster
- Valid GCP Service Account credential file (in JSON format)
- Project ID of the project in which the cluster is deployed
- Region in which the cluster is deployed
- Permissions to get GKE credentials (within the cluster permissions to get Roles, ClusterRoles, RoleBindings, ClusterRoleBindings and Namespaces are required)
- Audit logging configured for the cluster (Enabled by default, `GKE->Clusters->Cluster->Features->Logging`) and permissions to retrieve the logs
- For the collect_workloads feature (optional), permissions to retrieve workloads within the cluster are required

#### Local
- Name of the target cluster
- A valid KubeConfig file located at `~/.kube/config`
- Cluster permissions: get on Roles, ClusterRoles, RoleBindings, ClusterRoleBindings and Namespaces
- A valid Audit Log file in the standard Kubernetes format (for more information: https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
- For the collect_workloads feature (optional), permissions to retrieve workloads within the cluster are required


## Basic queries
### Database Structure
The main table (permission) is structured with the following fields: 
- `entity_name` - Name of the entity with the permission
- `entity_type` - Type of the entity with the permission
- `api_group` - API Group of the resource
- `resource_type` - The resource or subresource type
- `verb` - The action
- `permission_scope` - cluster-wide, resourceName, namespace or namespace/resourceName
- `permission_source` - The name of the permission grantor
- `permission_source_type` - The type of grantor (Role, ClusterRole, EKS Access Policy or Group)
- `permission_binding` - The name of the binding. When permission_source is a group, this is the object that binds the permissions to the group
- `permission_binding_type` - The type of binding (RoleBinding, ClusterRoleBinding, EKS Access Entry)
- `last_used_time` - Timestamp of the last usage of the permission within the examined timespan
- `last_used_resource` - The resource on which the permission was last used within the examined timespan

The second table (workload_identities) is structured with the following fields:
- `workload_type` - Type of the workload
- `workload_name` - Name of the workload
- `service_account_name` - Name of the ServiceAccount used by the workload
- `original_owner_type` - Type of the owner object of the workload (taken from ownerReferences). In standalone cases or owner objects, this will be the same type as the original workload
- `original_owner_name` - Name of the owner object of the workload (taken from ownerReferences). In standalone cases or owner objects, this will be the same name as the original workload


### Query examples (more complex queries can be seen in the advise() function under cloud_collect.go)
#### Get all permissions for AWS entities:
```select * from permission where entity_name REGEXP '^arn';```

#### Get all permissions for AAD entities (entity_name == objectID):
```select * from permission where entity_name REGEXP '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$';```

#### Get all permissions which have a record of being used in the observed timespan (7 days by default):
```select * from permission where last_used_time is not null;```

#### Get all permissions that have a record and haven't been used for X amount of time:
```select * from permission WHERE TIMESTAMPDIFF(HOUR, last_used_time, NOW()) > X*24;```

#### Get all members of a group:
```select entity_name from permission where permission_source = "<group-name>" group by entity_name;```

#### Get all entities who's permissions originate at a certain binding:
```select entity_name from permission where permission_binding = "<binding-name>" group by entity_name;```

#### Get all standalone workloads that use a ServiceAccount
```select workload_type, workload_name, service_account_name from workload_identities where workload_type=original_owner_type and workload_name=original_owner_name;```

#### Get a list of ServiceAccounts that are used by workloads and all of the workloads that use them
```SELECT service_account_name, GROUP_CONCAT(CONCAT(workload_type, ':', workload_name) ORDER BY workload_type, workload_name SEPARATOR ', ') as workloads FROM rufus.workload_identities GROUP BY service_account_name ORDER BY service_account_name;```

#### Get all of the entities in the cluster by order of the amount of individual permissions they have
```select entity_name, entity_type, count(*) as number_of_permissions from permission group by entity_name, entity_type ORDER BY number_of_permissions;```

#### Get a list of ServiceAccounts that are used by workloads and all of the workloads that use them by order of the amount of individual permissions they have
```SELECT a.service_account_name, a.workloads, b.number_of_permissions FROM(SELECT service_account_name, GROUP_CONCAT(CONCAT(workload_type, ':', workload_name) ORDER BY workload_type, workload_name SEPARATOR ', ') as workloads FROM rufus.workload_identities GROUP BY service_account_name ORDER BY service_account_name) AS a JOIN (select entity_name, count(*) as number_of_permissions from permission group by entity_name) AS b ON a.service_account_name = b.entity_name ORDER BY b.number_of_permissions;```

#### Get all entities with potentially risky permissions
``` SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'Wide secret access permissions' AS risk_reason, last_used_time FROM permission WHERE resource_type = 'secrets' AND verb IN('get', 'list') GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time UNION ALL SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'nodes/proxy access permissions' AS risk_reason, last_used_time FROM permission WHERE resource_type = 'nodes/proxy' AND verb IN ('create', 'get') AND permission_scope = 'cluster-wide' GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time HAVING COUNT(DISTINCT verb) = 2 UNION ALL SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'serviceaccount token creation permissions' AS risk_reason, last_used_time FROM permission WHERE resource_type = 'serviceaccounts/token' AND verb = 'create' GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time UNION ALL SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'Escalate, bind or impersonate permissions' AS risk_reason, last_used_time FROM permission WHERE verb IN('escalate', 'bind', 'impersonate') AND permission_scope = 'cluster-wide' GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time UNION ALL SELECT a.entity_name, a.entity_type, a.permission_source, a.permission_source_type, a.permission_binding, a.permission_binding_type, 'CSR and certificate issuing permissions' AS risk_reason, a.last_used_time FROM (SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time FROM permission WHERE resource_type = 'certificatesigningrequests' AND verb = 'create' AND permission_scope = 'cluster-wide' GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time) AS a INNER JOIN (SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time FROM permission WHERE resource_type = 'certificatesigningrequests/approval' AND verb IN ('patch', 'update') GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time) AS b ON a.entity_name = b.entity_name AND a.entity_type = b.entity_type AND a.permission_source = b.permission_source AND a.permission_source_type = b.permission_source_type AND a.permission_binding = b.permission_binding AND a.permission_binding_type = b.permission_binding_type UNION ALL SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'Workload creation permissions' AS risk_reason, last_used_time FROM permission WHERE resource_type IN ('pods', 'deployments', 'statefulsets', 'replicasets', 'daemonsets', 'jobs', 'cronjobs') AND verb = 'create' GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time UNION ALL SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'PersistentVolume creation permissions' AS risk_reason, last_used_time FROM permission WHERE resource_type = 'persistentvolumes' AND verb = 'create' AND permission_scope = 'cluster-wide' GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time UNION ALL SELECT entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, 'Admission webhook management permissions' AS risk_reason, last_used_time FROM permission WHERE resource_type IN ('validatingwebhookconfigurations', 'mutatingwebhookconfigurations') AND verb IN ('create', 'delete', 'patch', 'update') AND permission_scope = 'cluster-wide' GROUP BY entity_name, entity_type, permission_source, permission_source_type, permission_binding, permission_binding_type, last_used_time; ```

## General Information
In Kubernetes clusters, we rarely have full visibility into the permissions of each entity. Furthermore, we don't always know about all of the entities that have access altogether.
KIEMPossible aims to mitigate the majority of this problem, and allow you not only full visibility into the entities with access to your cluster and their permissions, but also to the usage of said permissions. This will allow you to make informed decisions regarding permissioning, allowing you to follow the principle of least privilege without having to worry about breaking workloads or blocking users, thus narrowing the attack surface for potential adversaries.
So what actually happens when you run KIEMPossible?
- Retrieval of all the Roles (refers to Roles and ClusterRoles) and Bindings (refers to RoleBindings and ClusterRoleBindings) in the cluster. If the `--collect-workloads` flag is set, retrieval of all of the workloads, ServiceAccounts they use and associated workload identities
- Extraction of all of the Subjects and their matching Roles from the Bindings
- "Flattening" the permissions for each subject to the lowest possible level (a single verb and scope - for namespaced resources this is either `namespace` or `namespace/resourceName`, for non-namespaced resources this is either `cluster-wide` or `resourceName`). For example, `*` on `pods` at the cluster level, becomes a line per verb applicable to the pods resource, per namespace in the cluster. This also takes into account special verbs which are only applicable to certain resources such as `impersonate` or `bind`. Additionally, top-level resources such as `serviceaccounts` are broken down to their subresources (so in this case the DB would end up with the relevant permissions for `serviceaccount` and for `serviceaccounts/token`). All of this "flattening" is crucial for the comparison of the permission table and the logs, allowing us to handle more specific cases
- Log ingestion based on the chosen provider. During the log ingestion, group inheritance is handled (check notes for GKE) - this means that users or serviceaccounts which don't get their permissions directly from Bindings but rather through group membership will be mapped to the DB. During this stage, we handle group inheritance for Local, AWS and AZURE clusters. Additionally, we handle EKS Access Entries in order to ensure coverage of entities with permissions gained through this method. Log ingestion is the stage where permissions in the database are mapped to actions taken within the cluster in order to determine the last usage of each permission within the given timeframe

#### Notes
There are still certain blind spots to which we must be vigilant:
- Logging is based on a policy. In self-managed cluster we can control what is logged and thereby control the visibility. In managed clusters, the CSPs control what is logged (for the most part the policy isn't visible to us). As such, there may be gaps in the last_used_time or last_used_resource fields in the DB depending on logging gaps (i.e some last_used_time or last_used_resource may be empty even if the action corresponding to the permission was performed)
- The last used time in the output DB is based on the timestamp that appears in the logs (this may be in a different timezone than your local timezone)
- A user who's permissions are gained through group inheritance and does not appear in the logs will not appear in the DB
- Logging happens at the API Server level, therfore direct interaction with the Kubelet will not appear in the DB
- Permissions the tool calculated through logs (Group inheritance and EKS Access Entries) may contain inaccuracies if the permissions were altered within the timeframe of the configured scan (7 days by default)
- EKS Access Entries for Service-Linked Roles are not currently supported
- For EKS, you will be prompted once your credentials expire to re-enter them in order for the tool to continue running
- The speed of log ingestion is limited to rate limiting set by the public cloud providers - while the values set worked best for the setup tested, you can modify these by changing the log "chunk" sizes in the code (`pkg/log_parsing/extract_aws.go`, `pkg/log_parsing/extract_azure.go`, and `pkg/log_parsing/extract_gcp.go`)
- GKE workload identity federation is not currently fully supported - currently only service accounts linked via annotations are supported
- In GCP, the Logging API has a relatively low rate limit. To tackle this, we set a high `pageSize` for each request sent - this is still not as fast as ingestion for the other cloud providers but works moderately well
- Lastly, in GKE logs, the `groups` claim is not displayed. As such, we do not (currently) support handling group inheritance for GKE, meaning the only permissions displayed are those we derive from the bindings within the cluster