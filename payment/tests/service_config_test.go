package tests

import (
	"testing"

	"encore.app/payment/handlers"
)

func TestBuildPaymentHandlerConfigRequiresReturnContextKey(t *testing.T) {
	_, err := handlers.NewPaymentConfig("https://pay.example.com", "protisan://payment-return", "")
	if err == nil {
		t.Fatal("expected missing ReturnContextKey to be rejected")
	}
}

func TestBuildPaymentHandlerConfigRequiresPublicBaseURL(t *testing.T) {
	_, err := handlers.NewPaymentConfig("", "protisan://payment-return", "secret")
	if err == nil {
		t.Fatal("expected missing PublicBaseURL to be rejected")
	}
}

func TestBuildPaymentHandlerConfigRejectsInvalidPublicBaseURL(t *testing.T) {
	_, err := handlers.NewPaymentConfig("pay.example.com", "protisan://payment-return", "secret")
	if err == nil {
		t.Fatal("expected invalid PublicBaseURL to be rejected")
	}
}

func TestBuildPaymentHandlerConfigRejectsHTTPPublicBaseURL(t *testing.T) {
	_, err := handlers.NewPaymentConfig("http://pay.example.com", "protisan://payment-return", "secret")
	if err == nil {
		t.Fatal("expected http PublicBaseURL to be rejected")
	}
}

func TestBuildPaymentHandlerConfigRejectsLocalPublicBaseURL(t *testing.T) {
	_, err := handlers.NewPaymentConfig("https://localhost:4000", "protisan://payment-return", "secret")
	if err == nil {
		t.Fatal("expected local PublicBaseURL to be rejected")
	}
}

func TestBuildPaymentHandlerConfigRequiresAppReturnURL(t *testing.T) {
	_, err := handlers.NewPaymentConfig("https://pay.example.com", "", "secret")
	if err == nil {
		t.Fatal("expected missing AppReturnURL to be rejected")
	}
}
