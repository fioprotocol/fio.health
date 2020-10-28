package fiohealth

import (
	tls "github.com/refraction-networking/utls"
	"log"
	"net"
	"net/url"
	"strings"
	"time"
)

// TestTls looks for old TLS versions and weak cipher suites, this is not the standard Go tls library, and it does
// not attempt to validate the certificate, knowing the settings are weak outside of having an invalid cert are
// useful findings.
func TestTls(uri string, debug bool) (string, bool) {
	var start time.Time
	if debug {
		start = time.Now()
	}
	defer func() {
		if debug && time.Now().Sub(start).Seconds() > 30 {
			log.Printf("TLS checks for %s took %d seconds", uri, time.Now().Sub(start).Seconds())
		}
	}()
	if !strings.HasPrefix(uri, "https") {
		return "", false
	}
	parsed, _ := url.Parse(uri)
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	hostname := parsed.Host
	addr := parsed.Host + ":" + port
	check := func(versions []uint16, suites []uint16) string {
		for _, vers := range versions {
			config := tls.Config{
				ServerName:         hostname,
				InsecureSkipVerify: true,
			}
			dialConn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				log.Printf("net.DialTimeout error: %+v", err)
				return ""
			}
			uTlsConn := tls.UClient(dialConn, &config, tls.HelloCustom)

			min := vers
			if min == tls.VersionTLS13 {
				// 1.3 can't be lowest, not allowed by utls lib.
				min = tls.VersionTLS12
			}
			// most of this taken from the utls example:
			spec := tls.ClientHelloSpec{
				TLSVersMax:   vers,
				TLSVersMin:   min,
				CipherSuites: suites,
				Extensions: []tls.TLSExtension{
					&tls.SNIExtension{},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{tls.X25519, tls.CurveP256}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{0}}, // uncompressed
					&tls.SessionTicketExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{"myFancyProtocol", "http/1.1"}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithSHA1,
						tls.PKCS1WithSHA1}},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519},
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{1}}, // pskModeDHE
					&tls.SupportedVersionsExtension{Versions: []uint16{vers}},
				},
				GetSessionID: nil,
			}
			err = uTlsConn.ApplyPreset(&spec)
			if err != nil {
				continue
			}
			_ = uTlsConn.SetDeadline(time.Now().Add(2 * time.Second))
			_ = uTlsConn.Handshake()
			var result string
			if uTlsConn.HandshakeState.ServerHello != nil {
				switch uTlsConn.ConnectionState().Version {
				case tls.VersionTLS10:
					result = "TLS v1.0"
				case tls.VersionTLS11:
					result = "TLS v1.1"
					//case tls.VersionTLS12:
					//	result = "TLS 1.2"
					//case tls.VersionTLS13:
					//	result = "TLS 1.3"
				}
				switch uTlsConn.ConnectionState().CipherSuite {
				case tls.TLS_RSA_WITH_RC4_128_SHA:
					result += " TLS_RSA_WITH_RC4_128_SHA"
				case tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:
					result += " TLS_RSA_WITH_3DES_EDE_CBC_SHA"
				case tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA:
					result += " TLS_ECDHE_ECDSA_WITH_RC4_128_SHA"
				case tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA:
					result += " TLS_ECDHE_RSA_WITH_RC4_128_SHA"
				case tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA:
					result += " TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA"
				}
				if result == "" {
					continue
				}
				_ = uTlsConn.Close()
				return result
			}
			_ = uTlsConn.Close()
		}
		return ""
	}

	findings := check(oldVersion, weakSuites)
	if findings != "" {
		return findings, true
	}
	findings = check(oldVersion, allSuites)
	if findings != "" {
		return findings, true
	}
	findings = check(anyVersion, weakSuites)
	if findings != "" {
		return findings, true
	}

	return "", false
}

var (
	anyVersion = []uint16{tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13}
	// for now only complain about tls v1.0, even though 1.1's days are numbered.
	//oldVersion = []uint16{tls.VersionTLS10, tls.VersionTLS11}
	oldVersion = []uint16{tls.VersionTLS10}
)

var allSuites = []uint16{
	tls.TLS_RSA_WITH_RC4_128_SHA,
	tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
	tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	tls.TLS_AES_128_GCM_SHA256,
	tls.TLS_AES_256_GCM_SHA384,
	tls.TLS_CHACHA20_POLY1305_SHA256,
}

var weakSuites = []uint16{
	tls.TLS_RSA_WITH_RC4_128_SHA,
	tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
}
