// Derived in part from IPQuality at commit 0ee5f192fed70c04615852efba0e4b8bd43546c7.
// Attribution and modification details are retained in THIRD_PARTY_NOTICES.md.

package probe

import (
	"bufio"
	"context"
	_ "embed"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
)

const dnsBlacklistWorkers = 16

//go:embed data/dnsbl.txt
var dnsBlacklistData string

type mailService struct {
	Name   string
	Domain string
}

var mailServices = []mailService{
	{Name: "Gmail", Domain: "gmail.com"},
	{Name: "Outlook", Domain: "outlook.com"},
	{Name: "Yahoo", Domain: "yahoo.com"},
	{Name: "Apple", Domain: "me.com"},
	{Name: "QQ", Domain: "qq.com"},
	{Name: "MailRU", Domain: "mail.ru"},
	{Name: "AOL", Domain: "aol.com"},
	{Name: "GMX", Domain: "gmx.com"},
	{Name: "MailCOM", Domain: "mail.com"},
	{Name: "163", Domain: "163.com"},
	{Name: "Sohu", Domain: "sohu.com"},
	{Name: "Sina", Domain: "sina.com"},
}

type mailFinding struct {
	Port25       any
	Services     map[string]any
	DNSBlacklist dnsBlacklistFinding
}

type dnsBlacklistFinding struct {
	Total       any
	Clean       any
	Marked      any
	Blacklisted any
}

func (engine *nativeEngine) probeMail(ctx context.Context, target netip.Addr) mailFinding {
	result := mailFinding{Services: make(map[string]any, len(mailServices))}
	result.Port25 = engine.smtpAvailable(ctx, "smtp.mailgun.org:25")
	for _, service := range mailServices {
		result.Services[service.Name] = engine.mailServiceAvailable(ctx, service.Domain)
	}
	if target.Is4() {
		result.DNSBlacklist = probeDNSBlacklists(ctx, target)
	}
	return result
}

func (engine *nativeEngine) mailServiceAvailable(ctx context.Context, domain string) bool {
	records, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil {
		return false
	}
	sort.Slice(records, func(left, right int) bool {
		if records[left].Pref != records[right].Pref {
			return records[left].Pref < records[right].Pref
		}
		return records[left].Host < records[right].Host
	})
	for _, record := range records {
		host := strings.TrimSuffix(record.Host, ".")
		if engine.smtpAvailable(ctx, net.JoinHostPort(host, "25")) {
			return true
		}
	}
	return false
}

func (engine *nativeEngine) smtpAvailable(ctx context.Context, address string) bool {
	dialContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	connection, err := engine.dialEndpoint(dialContext, address)
	if err != nil {
		return false
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(4 * time.Second))
	banner, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil || !strings.HasPrefix(strings.TrimSpace(banner), "220") {
		return false
	}
	_, _ = connection.Write([]byte("QUIT\r\n"))
	return true
}

func probeDNSBlacklists(ctx context.Context, address netip.Addr) dnsBlacklistFinding {
	zones := uniqueDNSBlacklistZones(dnsBlacklistData)
	reversed := address.As4()
	prefix := strings.Join([]string{
		decimalByte(reversed[3]), decimalByte(reversed[2]), decimalByte(reversed[1]), decimalByte(reversed[0]),
	}, ".")
	const (
		clean uint8 = iota
		marked
		blacklisted
	)
	jobs := make(chan string)
	results := make(chan uint8)
	var workers sync.WaitGroup
	for range dnsBlacklistWorkers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for zone := range jobs {
				results <- classifyDNSBlacklist(ctx, prefix+"."+zone)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, zone := range zones {
			select {
			case jobs <- zone:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()
	cleanCount, markedCount, blacklistedCount := 0, 0, 0
	for result := range results {
		switch result {
		case clean:
			cleanCount++
		case marked:
			markedCount++
		case blacklisted:
			blacklistedCount++
		}
	}
	total := cleanCount + markedCount + blacklistedCount
	return dnsBlacklistFinding{
		Total: total, Clean: cleanCount, Marked: markedCount, Blacklisted: blacklistedCount,
	}
}

func classifyDNSBlacklist(ctx context.Context, query string) uint8 {
	addresses, err := net.DefaultResolver.LookupHost(ctx, query)
	if err != nil || len(addresses) == 0 {
		return 0
	}
	classification := uint8(0)
	for _, value := range addresses {
		if value == "127.0.0.2" {
			return 2
		}
		if !strings.HasPrefix(value, "127.255.255.") {
			classification = 1
		}
	}
	return classification
}

func uniqueDNSBlacklistZones(contents string) []string {
	seen := make(map[string]struct{})
	for _, line := range strings.Split(contents, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		seen[line] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for line := range seen {
		result = append(result, line)
	}
	sort.Strings(result)
	return result
}

func decimalByte(value byte) string {
	const digits = "0123456789"
	if value >= 100 {
		return string([]byte{digits[value/100], digits[(value/10)%10], digits[value%10]})
	}
	if value >= 10 {
		return string([]byte{digits[value/10], digits[value%10]})
	}
	return string(digits[value])
}
