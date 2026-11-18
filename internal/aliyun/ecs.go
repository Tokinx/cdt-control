package aliyun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ecsGlobalEndpoint = "https://ecs.aliyuncs.com/"
	ecsVersion        = "2014-05-26"
)

// RegionInfo describes an Alibaba Cloud ECS region.
type RegionInfo struct {
	RegionID       string `json:"region_id"`
	LocalName      string `json:"local_name"`
	RegionEndpoint string `json:"region_endpoint"`
}

// ECSInstanceInfo holds metadata and runtime state returned by DescribeInstances.
type ECSInstanceInfo struct {
	InstanceID         string   `json:"instance_id"`
	InstanceName       string   `json:"instance_name"`
	RegionID           string   `json:"region_id"`
	ZoneID             string   `json:"zone_id"`
	Status             string   `json:"status"`
	InstanceType       string   `json:"instance_type"`
	CPU                int      `json:"cpu"`
	MemoryMB           int      `json:"memory_mb"`
	OSName             string   `json:"os_name"`
	ChargeType         string   `json:"charge_type"`
	InternetChargeType string   `json:"internet_charge_type"`
	PublicIPs          []string `json:"public_ips"`
	PrivateIPs         []string `json:"private_ips"`
	ExpiredTime        string   `json:"expired_time"`
}

type describeRegionsResp struct {
	Regions struct {
		Region []struct {
			RegionID       string `json:"RegionId"`
			LocalName      string `json:"LocalName"`
			RegionEndpoint string `json:"RegionEndpoint"`
		} `json:"Region"`
	} `json:"Regions"`
}

type describeInstancesResp struct {
	TotalCount int `json:"TotalCount"`
	PageNumber int `json:"PageNumber"`
	PageSize   int `json:"PageSize"`
	Instances  struct {
		Instance []struct {
			InstanceID         string `json:"InstanceId"`
			InstanceName       string `json:"InstanceName"`
			RegionID           string `json:"RegionId"`
			ZoneID             string `json:"ZoneId"`
			Status             string `json:"Status"`
			InstanceType       string `json:"InstanceType"`
			CPU                int    `json:"Cpu"`
			Memory             int    `json:"Memory"`
			OSName             string `json:"OSName"`
			InstanceChargeType string `json:"InstanceChargeType"`
			InternetChargeType string `json:"InternetChargeType"`
			ExpiredTime        string `json:"ExpiredTime"`
			PublicIPAddress    struct {
				IPAddress []string `json:"IpAddress"`
			} `json:"PublicIpAddress"`
			EipAddress struct {
				IPAddress string `json:"IpAddress"`
			} `json:"EipAddress"`
			VpcAttributes struct {
				PrivateIPAddress struct {
					IPAddress []string `json:"IpAddress"`
				} `json:"PrivateIpAddress"`
			} `json:"VpcAttributes"`
			InnerIPAddress struct {
				IPAddress []string `json:"IpAddress"`
			} `json:"InnerIpAddress"`
		} `json:"Instance"`
	} `json:"Instances"`
}

func ecsRegionalEndpoint(regionID string) string {
	r := strings.TrimSpace(regionID)
	if r == "" {
		return ecsGlobalEndpoint
	}
	return fmt.Sprintf("https://ecs.%s.aliyuncs.com/", r)
}

// DescribeRegions lists available ECS regions for the given credentials.
func (c *Client) DescribeRegions(ctx context.Context, creds Credentials) ([]RegionInfo, error) {
	var resp describeRegionsResp
	err := c.CallRPC(ctx, creds, ecsGlobalEndpoint, ecsVersion, "DescribeRegions", map[string]string{
		"AcceptLanguage": "zh-CN",
	}, &resp)
	if err != nil {
		return nil, err
	}
	out := make([]RegionInfo, 0, len(resp.Regions.Region))
	for _, r := range resp.Regions.Region {
		out = append(out, RegionInfo{
			RegionID:       r.RegionID,
			LocalName:      r.LocalName,
			RegionEndpoint: r.RegionEndpoint,
		})
	}
	return out, nil
}

// DescribeInstances fetches ECS instances in a region, optionally filtered by instanceIDs.
func (c *Client) DescribeInstances(ctx context.Context, creds Credentials, regionID string, instanceIDs []string) ([]ECSInstanceInfo, error) {
	endpoint := ecsRegionalEndpoint(regionID)

	if len(instanceIDs) > 0 {
		var result []ECSInstanceInfo
		for i := 0; i < len(instanceIDs); i += 100 {
			end := i + 100
			if end > len(instanceIDs) {
				end = len(instanceIDs)
			}
			idsJSON, _ := json.Marshal(instanceIDs[i:end])
			pageItems, _, err := c.describeInstancesPage(ctx, creds, endpoint, regionID, string(idsJSON), 1, 100)
			if err != nil {
				return nil, err
			}
			result = append(result, pageItems...)
		}
		return result, nil
	}

	var all []ECSInstanceInfo
	for page := 1; page <= 20; page++ {
		items, total, err := c.describeInstancesPage(ctx, creds, endpoint, regionID, "", page, 100)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(all) >= total || len(items) < 100 {
			break
		}
	}
	return all, nil
}

// DiscoverAllInstances discovers ECS instances across all available regions for the credentials.
func (c *Client) DiscoverAllInstances(ctx context.Context, creds Credentials) ([]ECSInstanceInfo, error) {
	regions, err := c.DescribeRegions(ctx, creds)
	if err != nil {
		return nil, err
	}
	if len(regions) == 0 {
		return nil, nil
	}

	type regionRes struct {
		items []ECSInstanceInfo
		err   error
	}

	regionCh := make(chan string, len(regions))
	for _, r := range regions {
		if id := strings.TrimSpace(r.RegionID); id != "" {
			regionCh <- id
		}
	}
	close(regionCh)

	workers := min(10, len(regions))
	if workers < 1 {
		workers = 1
	}

	resCh := make(chan regionRes, len(regions))
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for regionID := range regionCh {
				regCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
				items, err := c.DescribeInstances(regCtx, creds, regionID, nil)
				cancel()
				resCh <- regionRes{items: items, err: err}
			}
		}()
	}
	wg.Wait()
	close(resCh)

	var all []ECSInstanceInfo
	seen := make(map[string]struct{})
	successRegions := 0
	var firstErr error
	var authErr error

	for res := range resCh {
		if res.err != nil {
			if firstErr == nil {
				firstErr = res.err
			}
			var apiErr *APIError
			if authErr == nil && errors.As(res.err, &apiErr) {
				if apiErr.HTTPStatus == http.StatusUnauthorized || apiErr.HTTPStatus == http.StatusForbidden ||
					strings.HasPrefix(apiErr.Code, "InvalidAccessKeyId") ||
					apiErr.Code == "SignatureDoesNotMatch" ||
					strings.HasPrefix(apiErr.Code, "Forbidden") {
					authErr = res.err
				}
			}
			continue
		}
		successRegions++
		for _, it := range res.items {
			key := it.RegionID + ":" + it.InstanceID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			all = append(all, it)
		}
	}

	if successRegions == 0 && firstErr != nil {
		if authErr != nil {
			return nil, authErr
		}
		return nil, firstErr
	}

	slices.SortFunc(all, func(a, b ECSInstanceInfo) int {
		if a.RegionID != b.RegionID {
			return strings.Compare(a.RegionID, b.RegionID)
		}
		if a.InstanceName != b.InstanceName {
			return strings.Compare(a.InstanceName, b.InstanceName)
		}
		return strings.Compare(a.InstanceID, b.InstanceID)
	})
	return all, nil
}

func (c *Client) describeInstancesPage(
	ctx context.Context,
	creds Credentials,
	endpoint string,
	regionID string,
	instanceIDsJSON string,
	pageNumber int,
	pageSize int,
) ([]ECSInstanceInfo, int, error) {
	params := map[string]string{
		"RegionId":   regionID,
		"PageNumber": strconv.Itoa(pageNumber),
		"PageSize":   strconv.Itoa(pageSize),
	}
	if instanceIDsJSON != "" {
		params["InstanceIds"] = instanceIDsJSON
	}

	var resp describeInstancesResp
	if err := c.CallRPC(ctx, creds, endpoint, ecsVersion, "DescribeInstances", params, &resp); err != nil {
		return nil, 0, err
	}

	out := make([]ECSInstanceInfo, 0, len(resp.Instances.Instance))
	for _, inst := range resp.Instances.Instance {
		pub := make([]string, 0, len(inst.PublicIPAddress.IPAddress)+1)
		pub = append(pub, inst.PublicIPAddress.IPAddress...)
		if inst.EipAddress.IPAddress != "" {
			pub = append(pub, inst.EipAddress.IPAddress)
		}
		priv := make([]string, 0, len(inst.VpcAttributes.PrivateIPAddress.IPAddress)+len(inst.InnerIPAddress.IPAddress))
		priv = append(priv, inst.VpcAttributes.PrivateIPAddress.IPAddress...)
		priv = append(priv, inst.InnerIPAddress.IPAddress...)

		out = append(out, ECSInstanceInfo{
			InstanceID:         inst.InstanceID,
			InstanceName:       inst.InstanceName,
			RegionID:           inst.RegionID,
			ZoneID:             inst.ZoneID,
			Status:             inst.Status,
			InstanceType:       inst.InstanceType,
			CPU:                inst.CPU,
			MemoryMB:           inst.Memory,
			OSName:             inst.OSName,
			ChargeType:         inst.InstanceChargeType,
			InternetChargeType: inst.InternetChargeType,
			PublicIPs:          dedupStrings(pub),
			PrivateIPs:         dedupStrings(priv),
			ExpiredTime:        inst.ExpiredTime,
		})
	}
	return out, resp.TotalCount, nil
}

// StartInstances starts one or more ECS instances in the given region.
func (c *Client) StartInstances(ctx context.Context, creds Credentials, regionID string, instanceIDs []string) error {
	if len(instanceIDs) == 0 {
		return nil
	}
	params := map[string]string{
		"RegionId": regionID,
	}
	for i, id := range instanceIDs {
		params[fmt.Sprintf("InstanceId.%d", i+1)] = id
	}
	return c.CallRPC(ctx, creds, ecsRegionalEndpoint(regionID), ecsVersion, "StartInstances", params, nil)
}

// StopInstances stops one or more ECS instances in the given region.
func (c *Client) StopInstances(ctx context.Context, creds Credentials, regionID string, instanceIDs []string, forceStop bool, stoppedMode string) error {
	if len(instanceIDs) == 0 {
		return nil
	}
	params := map[string]string{
		"RegionId":  regionID,
		"ForceStop": strconv.FormatBool(forceStop),
	}
	if stoppedMode == "KeepCharging" || stoppedMode == "StopCharging" {
		params["StoppedMode"] = stoppedMode
	}
	for i, id := range instanceIDs {
		params[fmt.Sprintf("InstanceId.%d", i+1)] = id
	}
	return c.CallRPC(ctx, creds, ecsRegionalEndpoint(regionID), ecsVersion, "StopInstances", params, nil)
}

func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
