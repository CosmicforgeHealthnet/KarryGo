// Package ratelimit keeps Vendor Service's established import path while the
// canonical Redis implementation lives in shared/go for reuse by Analytics.
package ratelimit

import sharedratelimit "cosmicforge/logistics/shared/go/ratelimit"

type Limiter = sharedratelimit.Limiter

var New = sharedratelimit.New
