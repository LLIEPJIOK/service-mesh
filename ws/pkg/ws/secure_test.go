package ws_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LLIEPJIOK/service-mesh/ws/pkg/ws"
)

// generateTestCA создаёт CA сертификат для тестов
func generateTestCA(t *testing.T) (caCertPEM, caKeyPEM []byte, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
			CommonName:   "Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create CA certificate: %v", err)
	}

	caCert, _ = x509.ParseCertificate(caCertDER)

	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	caKeyDER, _ := x509.MarshalECPrivateKey(caKey)
	caKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyDER})

	return caCertPEM, caKeyPEM, caCert, caKey
}

// generateSignedCert создаёт сертификат, подписанный CA
func generateSignedCert(t *testing.T, id string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) (certPEM, keyPEM []byte) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   id,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(privateKey)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM
}

// generateSelfSignedCert создаёт самоподписанный сертификат
func generateSelfSignedCert(t *testing.T, id string) (certPEM, keyPEM []byte) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   id,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&privateKey.PublicKey,
		privateKey,
	)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return certPEM, keyPEM
}

// testPKI содержит CA и сгенерированные сертификаты для тестов
type testPKI struct {
	caCertPEM []byte
	caCert    *x509.Certificate
	caKey     *ecdsa.PrivateKey
	rootCAs   *x509.CertPool
}

func newTestPKI(t *testing.T) *testPKI {
	t.Helper()

	caCertPEM, _, caCert, caKey := generateTestCA(t)
	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(caCertPEM)

	return &testPKI{
		caCertPEM: caCertPEM,
		caCert:    caCert,
		caKey:     caKey,
		rootCAs:   rootCAs,
	}
}

func (pki *testPKI) generateCert(t *testing.T, id string) (certPEM, keyPEM []byte) {
	return generateSignedCert(t, id, pki.caCert, pki.caKey)
}

func setupSecureTestServer(t *testing.T, pki *testPKI) (*ws.SecureServer, *httptest.Server, *ws.TLSConfig) {
	t.Helper()

	serverCertPEM, serverKeyPEM := pki.generateCert(t, "server")

	tlsCfg := &ws.TLSConfig{
		CertificatePEM: serverCertPEM,
		PrivateKeyPEM:  serverKeyPEM,
		RootCAs:        pki.rootCAs,
	}

	cfg := ws.DefaultSecureServerConfig()
	cfg.TLS = tlsCfg

	server := ws.NewSecureServer(cfg)

	server.Handle("echo", func(ctx context.Context, payload json.RawMessage) (any, error) {
		var req EchoRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return EchoResponse{Message: req.Message, Echo: true}, nil
	})

	server.Handle("add", func(ctx context.Context, payload json.RawMessage) (any, error) {
		var nums []int
		if err := json.Unmarshal(payload, &nums); err != nil {
			return nil, err
		}
		sum := 0
		for _, n := range nums {
			sum += n
		}
		return map[string]int{"sum": sum}, nil
	})

	ts := httptest.NewServer(server)
	return server, ts, tlsCfg
}

func TestSecureClientServer_BasicEcho(t *testing.T) {
	pki := newTestPKI(t)
	_, ts, _ := setupSecureTestServer(t, pki)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	clientCertPEM, clientKeyPEM := pki.generateCert(t, "client")

	cfg := ws.DefaultSecureClientConfig(wsURL)
	cfg.TLS = &ws.TLSConfig{
		CertificatePEM: clientCertPEM,
		PrivateKeyPEM:  clientKeyPEM,
		RootCAs:        pki.rootCAs,
	}
	client := ws.NewSecureClient(cfg)

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	var resp EchoResponse
	err := client.RequestTyped(ctx, "echo", EchoRequest{Message: "hello secure"}, &resp)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.Message != "hello secure" {
		t.Errorf("expected message 'hello secure', got '%s'", resp.Message)
	}
	if !resp.Echo {
		t.Error("expected echo to be true")
	}
}

func TestSecureClientServer_ConcurrentRequests(t *testing.T) {
	pki := newTestPKI(t)
	_, ts, _ := setupSecureTestServer(t, pki)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	clientCertPEM, clientKeyPEM := pki.generateCert(t, "client")

	cfg := ws.DefaultSecureClientConfig(wsURL)
	cfg.TLS = &ws.TLSConfig{
		CertificatePEM: clientCertPEM,
		PrivateKeyPEM:  clientKeyPEM,
		RootCAs:        pki.rootCAs,
	}
	client := ws.NewSecureClient(cfg)

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	const numRequests = 50
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			var resp EchoResponse
			msg := EchoRequest{Message: strings.Repeat("x", i+1)}
			err := client.RequestTyped(ctx, "echo", msg, &resp)
			if err != nil {
				errors <- err
				return
			}

			if resp.Message != msg.Message {
				errors <- ws.ErrServerError
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request error: %v", err)
	}
}

func TestSecureClientServer_MultipleRoutes(t *testing.T) {
	pki := newTestPKI(t)
	_, ts, _ := setupSecureTestServer(t, pki)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	clientCertPEM, clientKeyPEM := pki.generateCert(t, "client")

	cfg := ws.DefaultSecureClientConfig(wsURL)
	cfg.TLS = &ws.TLSConfig{
		CertificatePEM: clientCertPEM,
		PrivateKeyPEM:  clientKeyPEM,
		RootCAs:        pki.rootCAs,
	}
	client := ws.NewSecureClient(cfg)

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	// Тестируем echo
	var echoResp EchoResponse
	if err := client.RequestTyped(ctx, "echo", EchoRequest{Message: "test"}, &echoResp); err != nil {
		t.Fatalf("echo request failed: %v", err)
	}
	if echoResp.Message != "test" {
		t.Errorf("expected 'test', got '%s'", echoResp.Message)
	}

	// Тестируем add
	var addResp map[string]int
	if err := client.RequestTyped(ctx, "add", []int{1, 2, 3, 4, 5}, &addResp); err != nil {
		t.Fatalf("add request failed: %v", err)
	}
	if addResp["sum"] != 15 {
		t.Errorf("expected sum 15, got %d", addResp["sum"])
	}
}

func TestSecureClientServer_RouteNotFound(t *testing.T) {
	pki := newTestPKI(t)
	_, ts, _ := setupSecureTestServer(t, pki)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	clientCertPEM, clientKeyPEM := pki.generateCert(t, "client")

	cfg := ws.DefaultSecureClientConfig(wsURL)
	cfg.TLS = &ws.TLSConfig{
		CertificatePEM: clientCertPEM,
		PrivateKeyPEM:  clientKeyPEM,
		RootCAs:        pki.rootCAs,
	}
	client := ws.NewSecureClient(cfg)

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	_, err := client.Request(ctx, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent route")
	}

	if !strings.Contains(err.Error(), "route not found") {
		t.Errorf("expected 'route not found' error, got: %v", err)
	}
}

func TestSecureConn_MQVKeyExchange(t *testing.T) {
	pki := newTestPKI(t)

	// Генерируем ключи для обеих сторон
	aliceCertPEM, aliceKeyPEM := pki.generateCert(t, "alice")
	bobCertPEM, bobKeyPEM := pki.generateCert(t, "bob")

	// Создаем соединения
	alice, err := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: aliceCertPEM,
		PrivateKeyPEM:  aliceKeyPEM,
		RootCAs:        pki.rootCAs,
	}, true)
	if err != nil {
		t.Fatalf("failed to create alice conn: %v", err)
	}

	bob, err := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: bobCertPEM,
		PrivateKeyPEM:  bobKeyPEM,
		RootCAs:        pki.rootCAs,
	}, false)
	if err != nil {
		t.Fatalf("failed to create bob conn: %v", err)
	}

	// Alice создает init
	initMsg, err := alice.CreateHandshakeInit()
	if err != nil {
		t.Fatalf("failed to create handshake init: %v", err)
	}

	// Bob обрабатывает init и создает response
	responseMsg, err := bob.ProcessHandshakeInit(initMsg)
	if err != nil {
		t.Fatalf("failed to process handshake init: %v", err)
	}

	// Alice обрабатывает response
	if err := alice.ProcessHandshakeResponse(responseMsg); err != nil {
		t.Fatalf("failed to process handshake response: %v", err)
	}

	// Проверяем, что обе стороны завершили handshake
	if !alice.IsHandshakeDone() {
		t.Error("alice handshake not done")
	}
	if !bob.IsHandshakeDone() {
		t.Error("bob handshake not done")
	}

	// Проверяем шифрование/дешифрование
	testData := []byte(`{"test": "data", "value": 123}`)

	// Alice шифрует, Bob расшифровывает
	encrypted, err := alice.Encrypt(testData)
	if err != nil {
		t.Fatalf("alice encrypt failed: %v", err)
	}

	decrypted, err := bob.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("bob decrypt failed: %v", err)
	}

	if string(decrypted) != string(testData) {
		t.Errorf("decrypted data mismatch: got %s, want %s", decrypted, testData)
	}

	// Bob шифрует, Alice расшифровывает
	encrypted2, err := bob.Encrypt(testData)
	if err != nil {
		t.Fatalf("bob encrypt failed: %v", err)
	}

	decrypted2, err := alice.Decrypt(encrypted2)
	if err != nil {
		t.Fatalf("alice decrypt failed: %v", err)
	}

	if string(decrypted2) != string(testData) {
		t.Errorf("decrypted data mismatch: got %s, want %s", decrypted2, testData)
	}
}

func TestSecureConn_MessageEncryptDecrypt(t *testing.T) {
	pki := newTestPKI(t)

	aliceCertPEM, aliceKeyPEM := pki.generateCert(t, "alice")
	bobCertPEM, bobKeyPEM := pki.generateCert(t, "bob")

	alice, _ := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: aliceCertPEM,
		PrivateKeyPEM:  aliceKeyPEM,
		RootCAs:        pki.rootCAs,
	}, true)
	bob, _ := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: bobCertPEM,
		PrivateKeyPEM:  bobKeyPEM,
		RootCAs:        pki.rootCAs,
	}, false)

	initMsg, _ := alice.CreateHandshakeInit()
	responseMsg, _ := bob.ProcessHandshakeInit(initMsg)
	alice.ProcessHandshakeResponse(responseMsg)

	// Создаем тестовое сообщение
	msg, err := ws.NewRequest("test.route", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Alice шифрует
	encMsg, err := alice.EncryptMessage(msg)
	if err != nil {
		t.Fatalf("encrypt message failed: %v", err)
	}

	// Bob расшифровывает
	decMsg, err := bob.DecryptMessage(encMsg)
	if err != nil {
		t.Fatalf("decrypt message failed: %v", err)
	}

	if decMsg.ID != msg.ID {
		t.Errorf("ID mismatch: got %d, want %d", decMsg.ID, msg.ID)
	}
	if decMsg.Route != msg.Route {
		t.Errorf("Route mismatch: got %s, want %s", decMsg.Route, msg.Route)
	}
}

func TestSecureClient_MissingTLSConfig(t *testing.T) {
	pki := newTestPKI(t)
	_, ts, _ := setupSecureTestServer(t, pki)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	// Клиент без TLS конфига
	cfg := ws.DefaultSecureClientConfig(wsURL)
	client := ws.NewSecureClient(cfg)

	ctx := context.Background()
	err := client.Connect(ctx)

	if err == nil {
		client.Close()
		t.Fatal("expected error when TLS config is missing")
	}
}

func TestSecureConn_CertificateValidation(t *testing.T) {
	pki := newTestPKI(t)

	// Создаём сертификаты, подписанные CA
	aliceCertPEM, aliceKeyPEM := pki.generateCert(t, "alice")
	bobCertPEM, bobKeyPEM := pki.generateCert(t, "bob")

	// Создаём соединения с валидацией
	alice, err := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: aliceCertPEM,
		PrivateKeyPEM:  aliceKeyPEM,
		RootCAs:        pki.rootCAs,
	}, true)
	if err != nil {
		t.Fatalf("failed to create alice conn: %v", err)
	}

	bob, err := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: bobCertPEM,
		PrivateKeyPEM:  bobKeyPEM,
		RootCAs:        pki.rootCAs,
	}, false)
	if err != nil {
		t.Fatalf("failed to create bob conn: %v", err)
	}

	// Handshake должен пройти успешно
	initMsg, err := alice.CreateHandshakeInit()
	if err != nil {
		t.Fatalf("failed to create handshake init: %v", err)
	}

	responseMsg, err := bob.ProcessHandshakeInit(initMsg)
	if err != nil {
		t.Fatalf("failed to process handshake init: %v", err)
	}

	if err := alice.ProcessHandshakeResponse(responseMsg); err != nil {
		t.Fatalf("failed to process handshake response: %v", err)
	}

	// Проверяем ID
	if alice.PeerID() != "bob" {
		t.Errorf("expected peer ID 'bob', got '%s'", alice.PeerID())
	}
	if bob.PeerID() != "alice" {
		t.Errorf("expected peer ID 'alice', got '%s'", bob.PeerID())
	}
}

func TestSecureConn_UntrustedCertificate(t *testing.T) {
	pki := newTestPKI(t)

	// Создаём сертификат, подписанный CA
	aliceCertPEM, aliceKeyPEM := pki.generateCert(t, "alice")

	// Создаём самоподписанный сертификат (не подписан CA)
	bobCertPEM, bobKeyPEM := generateSelfSignedCert(t, "bob")

	// Alice доверяет только CA
	alice, _ := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: aliceCertPEM,
		PrivateKeyPEM:  aliceKeyPEM,
		RootCAs:        pki.rootCAs,
	}, true)

	// Bob использует самоподписанный сертификат и не проверяет CA (RootCAs = nil)
	bob, _ := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: bobCertPEM,
		PrivateKeyPEM:  bobKeyPEM,
	}, false)

	initMsg, _ := alice.CreateHandshakeInit()

	// Bob может обработать init (он не проверяет CA)
	responseMsg, err := bob.ProcessHandshakeInit(initMsg)
	if err != nil {
		t.Fatalf("bob should accept alice's cert: %v", err)
	}

	// Alice должна отклонить Bob's сертификат
	err = alice.ProcessHandshakeResponse(responseMsg)
	if err == nil {
		t.Fatal("expected error: alice should reject untrusted certificate")
	}
	if err != ws.ErrCertNotTrusted {
		t.Errorf("expected ErrCertNotTrusted, got: %v", err)
	}
}

func TestSecureConn_PeerIDMismatch(t *testing.T) {
	pki := newTestPKI(t)

	// Создаём сертификаты
	aliceCertPEM, aliceKeyPEM := pki.generateCert(t, "alice")
	bobCertPEM, bobKeyPEM := pki.generateCert(t, "bob")

	// Alice ожидает другой ID
	alice, _ := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: aliceCertPEM,
		PrivateKeyPEM:  aliceKeyPEM,
		RootCAs:        pki.rootCAs,
		ExpectedPeerID: "charlie", // Ожидаем charlie, получим bob
	}, true)

	bob, _ := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: bobCertPEM,
		PrivateKeyPEM:  bobKeyPEM,
		RootCAs:        pki.rootCAs,
	}, false)

	initMsg, _ := alice.CreateHandshakeInit()
	responseMsg, _ := bob.ProcessHandshakeInit(initMsg)

	// Alice должна отклонить - ID не совпадает
	err := alice.ProcessHandshakeResponse(responseMsg)
	if err == nil {
		t.Fatal("expected error: alice should reject peer ID mismatch")
	}
	if err != ws.ErrPeerIDMismatch {
		t.Errorf("expected ErrPeerIDMismatch, got: %v", err)
	}
}

func TestSecureConn_ExpectedPeerIDMatch(t *testing.T) {
	pki := newTestPKI(t)

	aliceCertPEM, aliceKeyPEM := pki.generateCert(t, "alice")
	bobCertPEM, bobKeyPEM := pki.generateCert(t, "bob")

	// Alice ожидает bob
	alice, _ := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: aliceCertPEM,
		PrivateKeyPEM:  aliceKeyPEM,
		RootCAs:        pki.rootCAs,
		ExpectedPeerID: "bob",
	}, true)

	bob, _ := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: bobCertPEM,
		PrivateKeyPEM:  bobKeyPEM,
		RootCAs:        pki.rootCAs,
		ExpectedPeerID: "alice",
	}, false)

	initMsg, _ := alice.CreateHandshakeInit()
	responseMsg, err := bob.ProcessHandshakeInit(initMsg)
	if err != nil {
		t.Fatalf("bob should accept alice: %v", err)
	}

	err = alice.ProcessHandshakeResponse(responseMsg)
	if err != nil {
		t.Fatalf("alice should accept bob: %v", err)
	}

	if alice.PeerID() != "bob" {
		t.Errorf("expected peer ID 'bob', got '%s'", alice.PeerID())
	}
}

func TestSecureConn_NoRootCAsSkipsValidation(t *testing.T) {
	// Если RootCAs не установлен, проверка цепочки пропускается
	aliceCertPEM, aliceKeyPEM := generateSelfSignedCert(t, "alice")
	bobCertPEM, bobKeyPEM := generateSelfSignedCert(t, "bob")

	// Без RootCAs - не проверяем цепочку
	alice, _ := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: aliceCertPEM,
		PrivateKeyPEM:  aliceKeyPEM,
	}, true)

	bob, _ := ws.NewSecureConn(&ws.TLSConfig{
		CertificatePEM: bobCertPEM,
		PrivateKeyPEM:  bobKeyPEM,
	}, false)

	initMsg, _ := alice.CreateHandshakeInit()
	responseMsg, err := bob.ProcessHandshakeInit(initMsg)
	if err != nil {
		t.Fatalf("bob should accept alice without validation: %v", err)
	}

	err = alice.ProcessHandshakeResponse(responseMsg)
	if err != nil {
		t.Fatalf("alice should accept bob without validation: %v", err)
	}

	// Handshake должен завершиться успешно
	if !alice.IsHandshakeDone() || !bob.IsHandshakeDone() {
		t.Error("handshake should be done")
	}
}

func TestSecureClientServer_WithExpectedPeerID(t *testing.T) {
	pki := newTestPKI(t)

	serverCertPEM, serverKeyPEM := pki.generateCert(t, "server")

	tlsCfg := &ws.TLSConfig{
		CertificatePEM: serverCertPEM,
		PrivateKeyPEM:  serverKeyPEM,
		RootCAs:        pki.rootCAs,
		ExpectedPeerID: "client",
	}

	cfg := ws.DefaultSecureServerConfig()
	cfg.TLS = tlsCfg
	server := ws.NewSecureServer(cfg)

	server.Handle("echo", func(ctx context.Context, payload json.RawMessage) (any, error) {
		var req EchoRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return EchoResponse{Message: req.Message, Echo: true}, nil
	})

	ts := httptest.NewServer(server)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	clientCertPEM, clientKeyPEM := pki.generateCert(t, "client")

	clientCfg := ws.DefaultSecureClientConfig(wsURL)
	clientCfg.TLS = &ws.TLSConfig{
		CertificatePEM: clientCertPEM,
		PrivateKeyPEM:  clientKeyPEM,
		RootCAs:        pki.rootCAs,
		ExpectedPeerID: "server",
	}
	client := ws.NewSecureClient(clientCfg)

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	var resp EchoResponse
	err := client.RequestTyped(ctx, "echo", EchoRequest{Message: "hello"}, &resp)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.Message != "hello" {
		t.Errorf("expected message 'hello', got '%s'", resp.Message)
	}
}
