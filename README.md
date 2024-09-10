# KIEMScanner

- docker-compose up -d
- mysql -u mysql -p -h 127.0.0.1



- Flattened - the verbs that only work for specific resources needs to be documented
- Subresources level is where we flatten to
- Docs - write what it accounts for - stuff like resourceNames, all permissions are individually handled (some may appear twice for the same source if given by different granters). Permissions given specifically to subresources are shown as such in the db
- Docs - inclusterconfig as default, fallback to kubeconfig - kubeconfig handling separate for each CSP
- Docs - groups per user in logs to get user permissions

- AWS - permission to get creds and logs
- Azure - local kubernetes accounts need to be enabled (for the fetching of the kubeconfig), permissions to get clusteradmin creds and get logs
- GCP - currently stores all the flattened permission, doesn't get logs yet (60 request limit, need to incorporate something like pub sub, bigquery)

- Only check stuff that goes through API server, no logs for direct kubelet interaction (nodes/proxy) - obviously if perms through group etc and no action done, no visibility

- Document what happens for each (local, AWS, Azure, GCP)

- auth_handling good to go code wise (no internal comments etc), kube-collection all good just need to check SA collection stuff

- Azure: 1 million per 5 minutes log ingestion,1 million per 10 minutes log processing
- AWS: 1 million per 10 minutes log ingestion, 1 million per 10 minutes log processing


- issue with users permissions gotten from logs/access entries etc - if they changed over the last x amount (x=ingestion span) there will be inaccuracies

- Challenges with adding users through group inheritance, azure aad blah blah. In AWS, access entries - username may not be known in access entry (variable) and not in bindings.


- Not currently supporting access entries for service-linked roles. All user-assignable access policies are supported

