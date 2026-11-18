package aliyun

import (
	"context"
	"sort"
	"strings"
	"time"
)

const (
	cdtEndpoint    = "https://cdt.aliyuncs.com/"
	cdtVersion     = "2021-08-13"
	bytesPerGiB    = 1024.0 * 1024.0 * 1024.0
)

// RegionTraffic represents cumulative monthly CDT usage for one business region.
type RegionTraffic struct {
	BusinessRegionID string  `json:"business_region_id"`
	ISPType          string  `json:"isp_type,omitempty"`
	TrafficBytes     int64   `json:"traffic_bytes"`
	TrafficGB        float64 `json:"traffic_gb"`
	IsDomestic       bool    `json:"is_domestic"`
}

// CDTSummary aggregates domestic and overseas CDT traffic for an Aliyun account.
type CDTSummary struct {
	DomesticBytes int64           `json:"domestic_bytes"`
	OverseasBytes int64           `json:"overseas_bytes"`
	DomesticGB    float64         `json:"domestic_gb"`
	OverseasGB    float64         `json:"overseas_gb"`
	Details       []RegionTraffic `json:"details"`
	FetchedAt     time.Time       `json:"fetched_at"`
}

type listCdtTrafficResponse struct {
	RequestID      string `json:"RequestId"`
	TrafficDetails []struct {
		BusinessRegionID string `json:"BusinessRegionId"`
		ISPType          string `json:"ISPType"`
		Traffic          int64  `json:"Traffic"`
	} `json:"TrafficDetails"`
}

// IsDomesticRegion follows CDT billing pool classification:
// mainland China regions (cn-*) excluding cn-hongkong and cn-taiwan.
func IsDomesticRegion(regionID string) bool {
	r := strings.ToLower(strings.TrimSpace(regionID))
	if r == "" || !strings.HasPrefix(r, "cn-") {
		return false
	}
	return r != "cn-hongkong" && r != "cn-taiwan"
}

// FlowTypeLabel returns "境内" or "境外" for a region ID.
func FlowTypeLabel(regionID string) string {
	if IsDomesticRegion(regionID) {
		return "境内"
	}
	return "境外"
}

// ListCdtInternetTraffic fetches current calendar-month CDT internet traffic usage.
func (c *Client) ListCdtInternetTraffic(ctx context.Context, creds Credentials) (*CDTSummary, error) {
	var resp listCdtTrafficResponse
	if err := c.CallRPC(ctx, creds, cdtEndpoint, cdtVersion, "ListCdtInternetTraffic", nil, &resp); err != nil {
		return nil, err
	}

	summary := &CDTSummary{
		Details:   make([]RegionTraffic, 0, len(resp.TrafficDetails)),
		FetchedAt: time.Now(),
	}

	for _, item := range resp.TrafficDetails {
		domestic := IsDomesticRegion(item.BusinessRegionID)
		if domestic {
			summary.DomesticBytes += item.Traffic
		} else {
			summary.OverseasBytes += item.Traffic
		}
		summary.Details = append(summary.Details, RegionTraffic{
			BusinessRegionID: item.BusinessRegionID,
			ISPType:          item.ISPType,
			TrafficBytes:     item.Traffic,
			TrafficGB:        float64(item.Traffic) / bytesPerGiB,
			IsDomestic:       domestic,
		})
	}

	summary.DomesticGB = float64(summary.DomesticBytes) / bytesPerGiB
	summary.OverseasGB = float64(summary.OverseasBytes) / bytesPerGiB

	sort.Slice(summary.Details, func(i, j int) bool {
		return summary.Details[i].TrafficBytes > summary.Details[j].TrafficBytes
	})

	return summary, nil
}
