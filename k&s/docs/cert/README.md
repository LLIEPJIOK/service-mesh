# Сертификаты

## Корневой сертификат

### Описание

Корневой сертификат является основой для всей инфраструктуры сертификатов. Он используется для подписания промежуточных сертификатов и является доверенным источником для проверки подлинности сертификатов, выданных в рамках данной инфраструктуры.

### Генерация и использование корневого сертификата

Генерация корневого ключа и сертификата:

```bash
openssl genrsa -out ca.key 2048

openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 -out ca.crt -subj "/CN=My Internal Root CA"
```

Добавление ключа в секреты и корневого сертификата в кластер через ConfigMap:

```bash
kubectl create secret tls root-ca --cert=path/to/ca.crt --key=path/to/ca.key

kubectl create configmap root-ca --from-file=ca.crt=path/to/ca.crt
```

> [!IMPORTANT]
> Приватный ключ корневого сертификата (ca.key) никогда не должен монтироваться в pod’ы приложений.

Монтирование ключа в cert-manager:

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: cert-manager
          volumeMounts:
            - name: root-ca-key
              mountPath: /etc/mesh/key/tls.key
              subPath: tls.key
              readOnly: true
      volumes:
        - name: root-ca-key
          secret:
            secretName: root-ca
```

Корневой сертификат распространяется во все pod’ы через ConfigMap и используется sidecar-компонентами для проверки входящих TLS-соединений:

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: sidecar
          volumeMounts:
            - name: root-ca
              mountPath: /etc/mesh/ca
              readOnly: true
      volumes:
        - name: root-ca
          configMap:
            name: root-ca
```

## Сертификаты для сервисов

### Описание

Сертификаты для сервисов используются для обеспечения безопасности при взаимодействии между сервисами в кластере. Они позволяют аутентифицировать сервисы и обеспечивают шифрование данных при передаче.

### Генерация сертификатов для сервисов

Сервис сначала генерирует закрытый ключ и запрос на сертификат (CSR):

```go
privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
if err != nil {
	panic(err)
}
```

После формируется запрос на сертификат (CSR):

```go
csrTemplate := x509.CertificateRequest{
	Subject: pkix.Name{
		CommonName:   "my-service",
		Organization: []string{"my-organization"},
	},
	DNSNames: []string{"my-service.default.svc.cluster.local"},
}

csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, privateKey)
if err != nil {
	panic(err)
}
```

> [!NOTE]
> Значения CommonName и DNSNames в CSR не используются для определения идентичности сервиса и могут быть проигнорированы cert-manager. CSR используется исключительно как носитель публичного ключа.

После этого CSR отправляется в cert-manager для получения сертификата, который будет использоваться сервисом для аутентификации и шифрования данных при взаимодействии с другими сервисами в кластере.

> [!Note]
> Полученный сертификат хранится локально в приложении, при перезапуске приложение должно запрашивать новый

### Выдача сертификатов

Менеджер сертификатов выполняет следующие шаги:

1. Валидирует JWT токен через API Kubernetes
2. Выполняет `TokenReview` и извлекает identity из claims service account токена (`namespace` + `serviceaccount name`)
3. Проверяет подпись CSR (csr.CheckSignature())
4. **Игнорирует identity-поля из CSR (CN/SAN) как источник истины**
5. Формирует сертификат на основе identity из токена
6. Подписывает сертификат корневым ключом

> [!Important]
> Источник identity для выдачи сертификата - только валидированный токен [сервисного аккаунта](../../docs/service/account/README.md#сервисный-аккаунт). CSR не должен определять identity сертификата.

Контракт API и коды ошибок cert-manager см. в [Менеджер сертификатов](../../mesh/certmanager/README.md#api).

Пример парсинга и подписания CSR:

```go
func signCSR(csrPEM []byte, caCertPEM []byte, caKeyPEM []byte, identity string) ([]byte, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		return nil, fmt.Errorf("не удалось декодировать CSR")
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, err
	}

	// Проверка подписи CSR
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("некорректная подпись CSR")
	}

	// Парсинг CA
	caCertBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, err
	}

	caKeyBlock, _ := pem.Decode(caKeyPEM)
	caKey, err := x509.ParsePKCS8PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	// Формирование сертификата (identity берётся из валидированного service account token)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	certTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: identity,
		},
		DNSNames: []string{identity},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(1, 0, 0),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, certTemplate, caCert, csr.PublicKey, caKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}), nil
}
```

## Trust model

Доверие в системе строится следующим образом:

- Kubernetes является источником идентичности (ServiceAccount)
- cert-manager валидирует JWT токены через Kubernetes API
- cert-manager подписывает сертификаты с использованием корневого ключа
- Sidecar доверяют корневому сертификату и проверяют входящие соединения

## Ограничения реализации

В рамках данной реализации:

- отсутствует ротация сертификатов
- используется единый корневой центр сертификации
- не реализована модель SPIFFE
- идентичность задаётся на уровне ServiceAccount
