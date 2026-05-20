package domain

import "time"

type RateLimitSettings struct {
	RPS                    int   `json:"rps"`
	RPM                    int   `json:"rpm"`
	RPH                    int   `json:"rph"`
	RPD                    int   `json:"rpd"`
	ConcurrentConnections  int   `json:"concurrentConnections"`
	ConnectionsPerSecond   int   `json:"connectionsPerSecond"`
	UploadBytesPerSecond   int64 `json:"uploadBytesPerSecond"`
	DownloadBytesPerSecond int64 `json:"downloadBytesPerSecond"`
	TotalBytesPerDay       int64 `json:"totalBytesPerDay"`
	SubnetIPv4Mask         int   `json:"subnetIPv4Mask"`
	SubnetIPv6Mask         int   `json:"subnetIPv6Mask"`
}

type RateLimitViolation struct {
	IP       string    `json:"ip"`
	Scope    string    `json:"scope"`
	Limit    string    `json:"limit"`
	Exceeded string    `json:"exceeded"`
	Time     time.Time `json:"time"`
	Reason   string    `json:"reason"`
}

type RateLimitLease struct {
	Keys []string
	Now  time.Time
}
