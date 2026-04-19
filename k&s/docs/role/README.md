# Роли

## Описание

Роли в Kubernetes определяют набор разрешений, которые могут быть назначены пользователям или сервисам. Они позволяют управлять доступом к ресурсам и операциям в кластере. Роли могут быть кластерными (ClusterRole) или пространственными (Role), в зависимости от области действия.

## Привязка ролей

Привязка ролей (RoleBinding) связывает роль с пользователем, группой или сервисным аккаунтом, предоставляя им разрешения, определенные в роли. Привязки ролей могут быть кластерными (ClusterRoleBinding) или пространственными (RoleBinding), в зависимости от области действия.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: read-pods-for-service-a
  namespace: app-namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: pod-reader
subjects:
  - kind: ServiceAccount
    name: service-account-a
    namespace: app-namespace
```

## Pod viewer

Роль Pod viewer предоставляет sidecar необходимые права для service discovery в namespace: чтение pod, service и endpointslice ресурсов.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pod-reader
  namespace: app-namespace
rules:
  - apiGroups: [""]
    resources: ["pods", "services"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["discovery.k8s.io"]
    resources: ["endpointslices"]
    verbs: ["get", "list", "watch"]
```

> [!NOTE]
> `services` и `endpointslices` обязательны для построения и поддержки маппинга `ClusterIP:port -> serviceName` в sidecar discovery-кэше.

## Роль для cert-manager

Для валидации service account токенов cert-manager использует Kubernetes API `TokenReview`. Для этого требуется кластерная роль с правом `create` на ресурс `tokenreviews`.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cert-manager-tokenreviewer
rules:
  - apiGroups: ["authentication.k8s.io"]
    resources: ["tokenreviews"]
    verbs: ["create"]
```

Привязка роли к service account cert-manager:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: cert-manager-tokenreviewer-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cert-manager-tokenreviewer
subjects:
  - kind: ServiceAccount
    name: cert-manager
    namespace: mesh-system
```

> [!IMPORTANT]
> Без этой роли cert-manager не сможет валидировать JWT токены через TokenReview и должен отклонять запросы на выпуск сертификатов.

## Роль для webhook

В MVP webhook выполняет только мутацию pod spec и не создает отдельные Kubernetes ресурсы (например, `ServiceAccount`). Поэтому дополнительных прав на `serviceaccounts create/get` для webhook не требуется.
