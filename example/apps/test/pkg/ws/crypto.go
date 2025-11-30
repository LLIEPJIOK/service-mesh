package ws

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
)

var (
	ErrHandshakeFailed = errors.New("handshake failed")
	ErrInvalidCert     = errors.New("invalid certificate")
	ErrInvalidKey      = errors.New("invalid public key")
	ErrCertNotTrusted  = errors.New("certificate not signed by trusted CA")
	ErrPeerIDMismatch  = errors.New("peer ID does not match expected")
)

const phiBitLength = 128

type TLSConfig struct {
	CertificatePEM []byte         // X.509 сертификат в PEM
	PrivateKeyPEM  []byte         // Приватный ключ в PEM
	RootCAs        *x509.CertPool // Доверенные CA для проверки сертификатов партнёра
	ExpectedPeerID string         // Ожидаемый ID партнёра (опционально)
}

type HandshakeMessage struct {
	EphemeralKey string `json:"ephemeral_key"` // Эфемерный ключ V в base64 (uncompressed)
	Certificate  string `json:"certificate"`   // X.509 сертификат в PEM (содержит ID и Q)
}

type EncryptedMessage struct {
	Nonce      string `json:"nonce"`      // Nonce для AES-GCM в base64
	Ciphertext string `json:"ciphertext"` // Зашифрованные данные в base64
}

type SecureConn struct {
	// Долговременные ключи
	staticPriv *ecdsa.PrivateKey // d
	staticPub  *ecdsa.PublicKey  // Q
	localID    string            // Id (из CommonName сертификата)
	certPEM    []byte            // Оригинальный сертификат в PEM

	// Эфемерные ключи (Ephemeral) - генерируются на каждую сессию
	ephemPriv *big.Int // u
	ephemPubX *big.Int // V.x
	ephemPubY *big.Int // V.y

	// Состояние
	curve         elliptic.Curve
	sharedKey     []byte
	aead          cipher.AEAD
	handshakeDone bool
	isClient      bool // true = Alice (init), false = Bob (resp)

	// Peer data
	peerID     string
	peerPubX   *big.Int         // V_peer.x
	peerPubY   *big.Int         // V_peer.y
	peerStatic *ecdsa.PublicKey // Q_peer

	// Валидация сертификатов
	rootCAs        *x509.CertPool // Доверенные CA
	expectedPeerID string         // Ожидаемый ID партнёра (опционально)
}

func NewSecureConn(cfg *TLSConfig, isClient bool) (*SecureConn, error) {
	if cfg == nil {
		return nil, errors.New("TLS config is required")
	}

	certBlock, _ := pem.Decode(cfg.CertificatePEM)
	if certBlock == nil {
		return nil, ErrInvalidCert
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}

	keyBlock, _ := pem.Decode(cfg.PrivateKeyPEM)
	if keyBlock == nil {
		return nil, ErrInvalidKey
	}

	privKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	if !privKey.PublicKey.Equal(cert.PublicKey) {
		return nil, errors.New("private key does not match certificate")
	}

	localID := cert.Subject.CommonName
	if localID == "" {
		return nil, errors.New("certificate must have CommonName as ID")
	}

	sc := &SecureConn{
		staticPriv:     privKey,
		staticPub:      &privKey.PublicKey,
		localID:        localID,
		certPEM:        cfg.CertificatePEM,
		curve:          elliptic.P256(),
		isClient:       isClient,
		rootCAs:        cfg.RootCAs,
		expectedPeerID: cfg.ExpectedPeerID,
	}

	return sc, nil
}

func (sc *SecureConn) CreateHandshakeInit() (*HandshakeMessage, error) {
	return sc.CreateHandshakeMessage()
}

func (sc *SecureConn) ProcessHandshakeInit(msg *HandshakeMessage) (*HandshakeMessage, error) {
	if err := sc.generateEphemeral(); err != nil {
		return nil, err
	}

	if err := sc.ProcessHandshakeMessage(msg); err != nil {
		return nil, err
	}

	return sc.CreateHandshakeMessage()
}

func (sc *SecureConn) ProcessHandshakeResponse(msg *HandshakeMessage) error {
	return sc.ProcessHandshakeMessage(msg)
}

func (sc *SecureConn) CreateHandshakeMessage() (*HandshakeMessage, error) {
	if sc.ephemPriv == nil {
		if err := sc.generateEphemeral(); err != nil {
			return nil, err
		}
	}

	vBytes := elliptic.Marshal(sc.curve, sc.ephemPubX, sc.ephemPubY)

	return &HandshakeMessage{
		EphemeralKey: base64.StdEncoding.EncodeToString(vBytes),
		Certificate:  string(sc.certPEM),
	}, nil
}

func (sc *SecureConn) Encrypt(plaintext []byte) ([]byte, error) {
	if !sc.handshakeDone {
		return nil, ErrHandshakeFailed
	}

	nonce := make([]byte, sc.aead.NonceSize())
	rand.Read(nonce)

	return sc.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (sc *SecureConn) Decrypt(ciphertext []byte) ([]byte, error) {
	if !sc.handshakeDone {
		return nil, ErrHandshakeFailed
	}

	ns := sc.aead.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("ciphertext too short")
	}

	nonce, msg := ciphertext[:ns], ciphertext[ns:]

	return sc.aead.Open(nil, nonce, msg, nil)
}

func (sc *SecureConn) EncryptMessage(msg *Message) (*EncryptedMessage, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	encrypted, err := sc.Encrypt(data)
	if err != nil {
		return nil, err
	}

	ns := sc.aead.NonceSize()

	return &EncryptedMessage{
		Nonce:      base64.StdEncoding.EncodeToString(encrypted[:ns]),
		Ciphertext: base64.StdEncoding.EncodeToString(encrypted[ns:]),
	}, nil
}

func (sc *SecureConn) DecryptMessage(encMsg *EncryptedMessage) (*Message, error) {
	nonce, err := base64.StdEncoding.DecodeString(encMsg.Nonce)
	if err != nil {
		return nil, err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encMsg.Ciphertext)
	if err != nil {
		return nil, err
	}

	combined := append(nonce, ciphertext...)

	data, err := sc.Decrypt(combined)
	if err != nil {
		return nil, err
	}

	var msg Message

	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

func (sc *SecureConn) PeerCertificate() *ecdsa.PublicKey {
	return sc.peerStatic
}

func (sc *SecureConn) IsHandshakeDone() bool {
	return sc.handshakeDone
}

func (sc *SecureConn) PeerID() string {
	return sc.peerID
}

func (sc *SecureConn) LocalID() string {
	return sc.localID
}

func (sc *SecureConn) verifyCertificate(cert *x509.Certificate) error {
	if sc.rootCAs != nil {
		opts := x509.VerifyOptions{
			Roots: sc.rootCAs,
			KeyUsages: []x509.ExtKeyUsage{
				x509.ExtKeyUsageClientAuth,
				x509.ExtKeyUsageServerAuth,
			},
		}

		if _, err := cert.Verify(opts); err != nil {
			return ErrCertNotTrusted
		}
	}

	if sc.expectedPeerID != "" {
		if cert.Subject.CommonName != sc.expectedPeerID {
			return ErrPeerIDMismatch
		}
	}

	return nil
}

// Шаг 1 (для Алисы) и Шаг 3 (для Боба): Генерация u и V
func (sc *SecureConn) generateEphemeral() error {
	priv, x, y, err := elliptic.GenerateKey(sc.curve, rand.Reader)
	if err != nil {
		return err
	}

	sc.ephemPriv = new(big.Int).SetBytes(priv)
	sc.ephemPubX = x
	sc.ephemPubY = y

	return nil
}

func (sc *SecureConn) ProcessHandshakeMessage(msg *HandshakeMessage) error {
	vBytes, err := base64.StdEncoding.DecodeString(msg.EphemeralKey)
	if err != nil {
		return ErrInvalidKey
	}

	px, py := elliptic.Unmarshal(sc.curve, vBytes)
	if px == nil {
		return ErrInvalidKey
	}

	sc.peerPubX = px
	sc.peerPubY = py

	certBlock, _ := pem.Decode([]byte(msg.Certificate))
	if certBlock == nil {
		return ErrInvalidCert
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return err
	}

	if err := sc.verifyCertificate(cert); err != nil {
		return err
	}

	sc.peerID = cert.Subject.CommonName
	if sc.peerID == "" {
		return errors.New("peer certificate must have CommonName as ID")
	}

	peerStatic, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return ErrInvalidKey
	}

	sc.peerStatic = peerStatic

	return sc.calculateSharedKey()
}

// Реализация математики MQV (шаги 5 и 6)
func (sc *SecureConn) calculateSharedKey() error {
	curve := sc.curve
	N := curve.Params().N // Порядок группы q

	// Формируем строки X_A и X_B
	// X_A = Id_A || Id_B || V_B
	// X_B = Id_B || Id_A || V_A

	var idA, idB string
	var VA_x, VA_y, VB_x, VB_y *big.Int

	if sc.isClient { // Алиса
		idA, idB = sc.localID, sc.peerID
		VA_x, VA_y = sc.ephemPubX, sc.ephemPubY
		VB_x, VB_y = sc.peerPubX, sc.peerPubY
	} else { // Боб
		idA, idB = sc.peerID, sc.localID
		VA_x, VA_y = sc.peerPubX, sc.peerPubY
		VB_x, VB_y = sc.ephemPubX, sc.ephemPubY
	}

	vABytes := elliptic.Marshal(curve, VA_x, VA_y)
	vBBytes := elliptic.Marshal(curve, VB_x, VB_y)

	// X_A = Id_A || Id_B || V_B
	XA := hashContext(idA, idB, vBBytes)
	// X_B = Id_B || Id_A || V_A
	XB := hashContext(idB, idA, vABytes)

	// phi(V_A, X_A)
	phiA := calculatePhi(curve, VA_x, VA_y, XA)
	// phi(V_B, X_B)
	phiB := calculatePhi(curve, VB_x, VB_y, XB)

	var s *big.Int

	if sc.isClient {
		// Алиса: s_A = (u_A - phi_A * d_A) mod q
		term := new(big.Int).Mul(phiA, sc.staticPriv.D)
		s = new(big.Int).Sub(sc.ephemPriv, term)
		s.Mod(s, N)
	} else {
		// Боб: s_B = (u_B - phi_B * d_B) mod q
		term := new(big.Int).Mul(phiB, sc.staticPriv.D)
		s = new(big.Int).Sub(sc.ephemPriv, term)
		s.Mod(s, N)
	}

	// T = V_peer - phi_peer * Q_peer
	var peerV_x, peerV_y, peerQ_x, peerQ_y, phi_peer *big.Int
	if sc.isClient {
		peerV_x, peerV_y = sc.peerPubX, sc.peerPubY
		peerQ_x, peerQ_y = sc.peerStatic.X, sc.peerStatic.Y
		phi_peer = phiB
	} else {
		peerV_x, peerV_y = sc.peerPubX, sc.peerPubY
		peerQ_x, peerQ_y = sc.peerStatic.X, sc.peerStatic.Y
		phi_peer = phiA
	}

	// 1. Calc temp = phi_peer * Q_peer
	tempX, tempY := curve.ScalarMult(peerQ_x, peerQ_y, phi_peer.Bytes())

	// 2. Negate temp (инверсия точки: (x, -y mod P))
	negTempY := new(big.Int).Sub(curve.Params().P, tempY)
	negTempY.Mod(negTempY, curve.Params().P)

	// 3. T = V_peer + (-temp)
	TX, TY := curve.Add(peerV_x, peerV_y, tempX, negTempY)

	// Итоговый K = s * T
	KX, _ := curve.ScalarMult(TX, TY, s.Bytes())

	if KX == nil {
		return errors.New("calculated point at infinity")
	}

	// Превращаем координату X общей точки в симметричный ключ (HKDF или просто SHA-256)
	// Для простоты примера берем SHA-256 от X
	h := sha256.Sum256(KX.Bytes())
	sc.sharedKey = h[:]

	// Инициализируем AES-GCM
	block, err := aes.NewCipher(sc.sharedKey)
	if err != nil {
		return err
	}

	sc.aead, err = cipher.NewGCM(block)
	if err != nil {
		return err
	}

	sc.handshakeDone = true
	return nil
}

func hashContext(id1, id2 string, vBytes []byte) []byte {
	h := sha256.New()
	h.Write([]byte(id1))
	h.Write([]byte(id2))
	h.Write(vBytes)

	return h.Sum(nil)
}

// Вспомогательная функция: phi(V, X) -> {2^l, ..., 2^{l+1}-1}
func calculatePhi(curve elliptic.Curve, Vx, Vy *big.Int, context []byte) *big.Int {
	h := sha256.New()
	h.Write(elliptic.Marshal(curve, Vx, Vy))
	h.Write(context)
	digest := h.Sum(nil)

	trunc := digest[:16]
	res := new(big.Int).SetBytes(trunc)

	// устанавливаем 129-й бит
	mod := new(big.Int).Lsh(big.NewInt(1), phiBitLength)
	res.Mod(res, mod)
	res.Add(
		res,
		mod,
	)

	return res
}
