#!/bin/bash

# Function to get all resource types
get_resource_types() {
  kubectl api-resources | awk '{print $NF}' > resource_types.txt
}

# Function to create a resource
create_resource() {
  resource_type=$1
  resource_name="kiem-$resource_type"

  case $resource_type in
    Pod)
      kubectl run $resource_name --image=nginx
      ;;
    Deployment)
      kubectl create deployment $resource_name --image=nginx
      ;;
    Secret)
      kubectl create secret generic $resource_name --from-literal=key1=supersecret
      ;;
    ConfigMap)
      kubectl create configmap $resource_name --from-literal=key1=value1
      ;;
    Role)
      kubectl create role $resource_name --verb=get --resource=pod
      ;;
    ClusterRole)
      kubectl create clusterrole $resource_name --verb=get --resource=pod
      ;;
    ClusterRoleBinding)
      kubectl create clusterrolebinding $resource_name --clusterrole=kiem-clusterrole
      ;;
    RoleBinding)
      kubectl create rolebinding $resource_name --role=kiem-role
      ;;
    CronJob)
      kubectl create cronjob $resource_name --image=nginx --schedule="*/1 * * * *"
      ;;
    Job)
      kubectl create job $resource_name --image=nginx
      ;;
    Namespace)
      kubectl create namespace $resource_name
      ;;
    PriorityClass)
      kubectl create priorityclass $resource_name
      ;;
    ResourceQuota)
      kubectl create quota $resource_name
      ;;
    Ingress)
      kubectl create ingress $resource_name --rule="foo.com/bar=svc1:8080"
      ;;
    PodDisruptionBudget)
      kubectl create poddisruptionbudget $resource_name --selector=kiem --min-available=1
      ;;
    Service)
      kubectl create service $resource_name --tcp=80
      ;;
    ServiceAccount)
      kubectl create serviceaccount $resource_name
      kubectl create token kiem-serviceaccount
      ;;
    Endpoint | LimitRange)
      cat <<EOF > ${resource_type}_template.yaml
apiVersion: v1
kind: $resource_type
metadata:
  name: $resource_name
  namespace: default
EOF
      kubectl apply -f ${resource_type}_template.yaml
      ;;
    PersistentVolume)
      cat <<EOF > ${resource_type}_template.yaml
apiVersion: v1
kind: $resource_type
metadata:
  name: $resource_name
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  storageClassName: standard
  hostPath:
    path: /tmp/test
EOF
      kubectl apply -f ${resource_type}_template.yaml
      ;;
    PodTemplate)
      cat <<EOF > ${resource_type}_template.yaml
apiVersion: v1
kind: $resource_type
metadata:
  name: $resource_name
  labels:
    kiem: test
template:
  metadata:
    labels:
      kiem: test
  spec:
    containers:
    - name: kiem
      image: nginx
EOF
      kubectl apply -f ${resource_type}_template.yaml
      ;;
    ReplicationController)
      cat <<EOF > ${resource_type}_template.yaml
apiVersion: v1
kind: $resource_type
metadata:
  name: $resource_name
  labels:
    kiem: test
spec:
  replicas: 1
  selector:
    kiem: test
  template:
    metadata:
      labels:
        kiem: test
    spec:
      containers:
      - name: kiem
        image: nginx
EOF
      kubectl apply -f ${resource_type}_template.yaml
      ;;
    PersistentVolumeClaim)
      cat <<EOF > ${resource_type}_template.yaml
apiVersion: v1
kind: $resource_type
metadata:
  name: $resource_name
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
EOF
      kubectl apply -f ${resource_type}_template.yaml
      ;;
    MutatingWebhookConfiguration)
      cat <<EOF > ${resource_type}_template.yaml
apiVersion: admissionregistration.k8s.io/v1
kind: $resource_type
metadata:
  name: $resource_name
webhooks:
  - name: foo.bar.test
    clientConfig:
      service:
        name: $resource_name
        namespace: default
        path: "/mutate"
    rules:
      - operations: ["CREATE"]
        apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
    admissionReviewVersions: ["v1"]
    sideEffects: None
    timeoutSeconds: 10
EOF
      kubectl apply -f ${resource_type}_template.yaml
      ;;
    ValidatingAdmissionPolicy)
      cat <<EOF > ${resource_type}_template.yaml
apiVersion: admissionregistration.k8s.io/v1
kind: $resource_type
metadata:
  name: $resource_name
  namespace: default
spec:
  paramKind:
    apiVersion: v1
    kind: ConfigMap
  matchConstraints:
    resourceRules:
      - apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
        operations: ["CREATE"]
  validations:
    - expression: "object.metadata.name.startsWith('valid-')"
EOF
      kubectl apply -f ${resource_type}_template.yaml
      ;;
    ValidatingAdmissionPolicyBinding)
      cat <<EOF > ${resource_type}_template.yaml
apiVersion: admissionregistration.k8s.io/v1
kind: $resource_type
metadata:
  name: $resource_name
spec:
  policyName: $resource_name
  paramRef:
    name: $resource_name
    parameterNotFoundAction: Allow
  matchResources:
    resourceRules:
      - apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
        operations: ["CREATE"]
  validationActions: ["Allow"]
EOF
      kubectl apply -f ${resource_type}_template.yaml
      ;;
    ValidatingWebhookConfiguration)
      cat <<EOF > ${resource_type}_template.yaml
apiVersion: admissionregistration.k8s.io/v1
kind: $resource_type
metadata:
  name: $resource_name
webhooks:
  - name: foo.bar.test
    clientConfig:
      service:
        name: $resource_name
        namespace: default
        path: /validate
    rules:
      - apiGroups: [""]
        apiVersions: ["v1"]
        operations: ["CREATE"]
        resources: ["pods"]
    admissionReviewVersions: ["v1"]
    sideEffects: None
EOF
      kubectl apply -f ${resource_type}_template.yaml
      ;;
    HorizontalPodAutoscaler)
      cat <<EOF > hpa_template.yaml
apiVersion: autoscaling/v1
kind: HorizontalPodAutoscaler
metadata:
  name: $resource_name
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-deployment
  minReplicas: 1
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80
EOF
      kubectl apply -f hpa_template.yaml
      ;;
    # Default case for unsupported resources
    *)
      echo "Resource type $resource_type is not supported for creation (at least via kubectl) or missing from this script."
      ;;
  esac
}

# Function to perform all possible verbs on a created resource
perform_verbs() {
  resource_type=$1
  resource_name="kiem-$resource_type"

  # Get all verbs for the resource
  kubectl api-resources -o wide | grep -w "$resource_type" | awk '{print $6}' > verbs.txt

  # Perform each verb
  while read verb; do
    if [[ "$verb" != "create" && "$verb" != "delete" ]]; then
      echo "Performing $verb on $resource_type $resource_name"
      kubectl $verb $resource_type $resource_name
    fi
  done < verbs.txt
}

# Function to delete a created resource
delete_resource() {
  resource_type=$1
  resource_name="kiem-$resource_type"

  case $resource_type in
    deployment|replicaset|statefulset|daemonset|job)
      # For workloads that might add random strings, use labels to delete
      kubectl delete $resource_type -l app=$resource_name
      ;;
    *)
      kubectl delete $resource_type $resource_name
      ;;
  esac
}

# Main function
main() {
  get_resource_types
  while read resource_type; do
    create_resource $resource_type
    perform_verbs $resource_type
    delete_resource $resource_type
  done < resource_types.txt
}

main




# just do get on all for weird resources (events)

# Can't do create - events, componentstatus, nodes




# Still need to do:
# CustomResourceDefinition
# APIService
# ControllerRevision
# DaemonSet
# ReplicaSet
# StatefulSet
# SelfSubjectReview
# TokenReview
# LocalSubjectAccessReview
# SelfSubjectAccessReview
# SelfSubjectRulesReview
# SubjectAccessReview
# CertificateSigningRequest
# Lease
# EndpointSlice
# FlowSchema
# PriorityLevelConfiguration
# IngressClass
# NetworkPolicy
# RuntimeClass
# CSIDriver
# CSINode
# CSIStorageCapacity
# StorageClass
# VolumeAttachment