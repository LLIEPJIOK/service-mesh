# Сервисный аккаунт

## Описание

В kubernetes сервисный аккаунт - это идентичность для подов, которую kubernetes API умеет аутентифицировать и проверять. В рамках mesh-сервиса сервисный аккаунт используется для аутентификации при запросе сертификатов в cert-manager.

## Базовый принцип

- Один логический сервис (обычно один Deployment) использует свой ServiceAccount.
- Sidecar отправляет JWT токен этого ServiceAccount в cert-manager.
- Cert-manager проверяет токен через TokenReview.

## Интеграция с cert-manager

При запросе сертификата sidecar передает токен service account в `POST /sign`. cert-manager:

1. валидирует токен через Kubernetes API `TokenReview`;
2. извлекает identity из claims токена;
3. выпускает сертификат на основе identity из токена, а не на основе полей CSR.

Спецификация API и требований находится в [Менеджер сертификатов](../../../mesh/certmanager/README.md).

## Создание сервисного аккаунта

В mesh MVP создание ресурса `ServiceAccount` выполняется манифестами приложения (или оператором), а webhook только патчит поле `spec.serviceAccountName` в pod template.

1. если `spec.serviceAccountName` уже задан в workload-манифесте, webhook его не меняет;
2. если поле пустое, webhook вычисляет имя service account по детерминированному алгоритму;
3. если имя вычислить не удалось, webhook использует `default`;
4. webhook не создает и не изменяет Kubernetes ресурс `ServiceAccount`.

Подробный admission-контракт см. в [Service mesh hook](../../../mesh/hook/README.md).

Создание сервисного аккаунта для деплоймента:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-service
  namespace: app-namespace
```

Использование сервисного аккаунта в деплойменте:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
  namespace: app-namespace
spec:
  template:
    spec:
      serviceAccountName: my-service
```

> [!IMPORTANT]
> Для предсказуемости identity рекомендуется явно задавать `serviceAccountName` в Deployment и создавать соответствующий `ServiceAccount` отдельным манифестом.

## Права доступа

Права доступа сервисного аккаунта определяются [ролями](../../role/README.md#роли) и [привязками ролей](../../role/README.md#привязка-ролей).

## Начало работы

При старте под автоматически получает следующее:

```
/var/run/secrets/kubernetes.io/serviceaccount/
  ├── token
  ├── ca.crt
  └── namespace
```

В частности токен содержит следующие поля, которые используются для создания сертификата в cert-manager:

```json
{
	"sub": "system:serviceaccount:default:{deployment-name}",
	"kubernetes.io/serviceaccount/name": "{deployment-name}",
	"kubernetes.io/serviceaccount/namespace": "default"
}
```
