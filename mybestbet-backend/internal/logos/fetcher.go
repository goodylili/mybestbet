package logos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

const (
	MaxImageBytes   = 512 << 10
	fetchTimeout    = 20 * time.Second
	maxRedirects    = 3
	fetchUserAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"
	fetchAcceptType = "image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8"
)

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
}

type dialGuard func(network, address string, conn syscall.RawConn) error

type Fetcher struct {
	client *http.Client
}

type fetchedImage struct {
	contentType string
	sha256      string
	bytes       []byte
}

func NewFetcher() *Fetcher { return newFetcher(refuseNonPublic) }

func newFetcher(guard dialGuard) *Fetcher {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: guard}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
	}
	client := &http.Client{
		Timeout:   fetchTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects: %w", maxRedirects, ErrUnsupportedURL)
			}
			return checkURL(req.URL)
		},
	}
	return &Fetcher{client: client}
}

func refuseNonPublic(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("dial address %q: %w", address, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("dial address %q: %w", address, err)
	}
	if !isPublic(ip.Unmap()) {
		return fmt.Errorf("%s: %w", ip, ErrBlockedAddress)
	}
	return nil
}

func isPublic(ip netip.Addr) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func checkURL(u *url.URL) error {
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("%q: %w", u.Redacted(), ErrUnsupportedURL)
	}
	return nil
}

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (fetchedImage, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return fetchedImage{}, fmt.Errorf("parse logo url: %w", ErrUnsupportedURL)
	}
	if err := checkURL(target); err != nil {
		return fetchedImage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fetchedImage{}, fmt.Errorf("build logo request: %w", err)
	}
	req.Header.Set("User-Agent", fetchUserAgent)
	req.Header.Set("Accept", fetchAcceptType)
	resp, err := f.client.Do(req)
	if err != nil {
		return fetchedImage{}, fmt.Errorf("get logo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fetchedImage{}, fmt.Errorf("status %d: %w", resp.StatusCode, ErrUpstreamStatus)
	}
	if resp.ContentLength > MaxImageBytes {
		return fetchedImage{}, fmt.Errorf("content-length %d: %w", resp.ContentLength, ErrTooLarge)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxImageBytes+1))
	if err != nil {
		return fetchedImage{}, fmt.Errorf("read logo: %w", err)
	}
	if len(body) > MaxImageBytes {
		return fetchedImage{}, fmt.Errorf("body over %d bytes: %w", MaxImageBytes, ErrTooLarge)
	}
	contentType, ok := sniffImage(body)
	if !ok {
		return fetchedImage{}, fmt.Errorf("sniffed %q: %w", http.DetectContentType(body), ErrNotImage)
	}
	sum := sha256.Sum256(body)
	return fetchedImage{contentType: contentType, sha256: hex.EncodeToString(sum[:]), bytes: body}, nil
}
