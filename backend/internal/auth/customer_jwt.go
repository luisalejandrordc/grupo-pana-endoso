package auth

import (
	"endosoflow/internal/platform/ids"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const accountTypeCustomer = "customer"

// IssueCustomerAccessToken signs a short-lived access token for a customer. It carries no role: customers have
// none, and employee code paths reject it because its account type is "customer" (and customer paths reject an
// employee's token for the same reason).
func IssueCustomerAccessToken(secret []byte, customerID uuid.UUID, now time.Time) (string, time.Time, error) {
	expires := now.Add(AccessTokenTTL)
	claims := tokenClaims{
		AccountType: accountTypeCustomer,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   customerID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
			ID:        ids.NewString(),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign customer access token: %w", err)
	}
	return signed, expires, nil
}

// ParseCustomerAccessToken verifies a customer's access token and returns the customer it names. Like its
// employee counterpart it checks signature, algorithm, issuer, expiry and shape, and additionally that the token
// was issued for a customer.
func ParseCustomerAccessToken(secret []byte, token string, now time.Time) (uuid.UUID, error) {
	parsed, err := jwt.ParseWithClaims(token, &tokenClaims{},
		func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(func() time.Time { return now }),
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	claims, ok := parsed.Claims.(*tokenClaims)
	if !ok || !parsed.Valid || claims.AccountType != accountTypeCustomer || claims.Role != "" {
		return uuid.Nil, ErrInvalidToken
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}
	return id, nil
}
