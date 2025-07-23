# CIS and Pod Security Admission

In order to set a custom Pod Security Admission policy when CIS profile is selected it's required to create a secret with the policy content and set an appropriate field on the `RKE2ControlPlane` object:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: pod-security-admission-config
data:
  pod-security-admission-config.yaml: |
    apiVersion: apiserver.config.k8s.io/v1
    kind: AdmissionConfiguration
    plugins:
    - name: PodSecurity
    configuration:
        apiVersion: pod-security.admission.config.k8s.io/v1beta1
        kind: PodSecurityConfiguration
        defaults:
        enforce: "restricted"
        enforce-version: "latest"
        audit: "restricted"
        audit-version: "latest"
        warn: "restricted"
        warn-version: "latest"
        exemptions:
        usernames: []
        runtimeClasses: []
        namespaces: [kube-system, cis-operator-system, tigera-operator]
```

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: RKE2ControlPlane
metadata:
  ...
spec:
  ...
  files:
    - path: /path/to/pod-security-admission-config.yaml
      contentFrom:
        secret:
          name: pod-security-admission-config
          key: pod-security-admission-config.yaml
  agentConfig:
    profile: cis
    podSecurityAdmissionConfigFile: /path/to/pod-security-admission-config.yaml
    ...
```

> **_NOTE:_**: You can also use a ConfigMap instead of a Secret for the above configuration by using `.contentFrom.configMap` instead of `.contentFrom.secret`.

## Example of PSA to allow Rancher components to run in the cluster:

```yaml 
apiVersion: apiserver.config.k8s.io/v1
kind: AdmissionConfiguration
plugins:
  - name: PodSecurity
    configuration:
      apiVersion: pod-security.admission.config.k8s.io/v1
      kind: PodSecurityConfiguration
      defaults:
        enforce: "restricted"
        enforce-version: "latest"
        audit: "restricted"
        audit-version: "latest"
        warn: "restricted"
        warn-version: "latest"
      exemptions:
        usernames: []
        runtimeClasses: []
        namespaces: [cattle-alerting,
                     cattle-fleet-local-system,
                     cattle-fleet-system,
                     cattle-global-data,
                     cattle-impersonation-system,
                     cattle-monitoring-system,
                     cattle-prometheus,
                     cattle-resources-system,
                     cattle-system,
                     cattle-ui-plugin-system,
                     cert-manager,
                     cis-operator-system,
                     fleet-default,
                     ingress-nginx,
                     kube-node-lease,
                     kube-public,
                     kube-system,
                     rancher-alerting-drivers]
```