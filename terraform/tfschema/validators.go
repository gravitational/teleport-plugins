package tfschema

import (
	"context"
	fmt "fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TimeValueInFutureValidator ensures that a time is in the future
type TimeValueInFutureValidator struct{}

// MustTimeBeInFuture returns TimeValueInFutureValidator
func MustTimeBeInFuture() tfsdk.AttributeValidator {
	return TimeValueInFutureValidator{}
}

// Description returns validator description
func (v TimeValueInFutureValidator) Description(_ context.Context) string {
	return "Checks that a time value is in future"
}

// MarkdownDescription returns validator markdown description
func (v TimeValueInFutureValidator) MarkdownDescription(_ context.Context) string {
	return "Checks that a time value is in future"
}

// Validate performs the validation.
func (v TimeValueInFutureValidator) Validate(_ context.Context, req tfsdk.ValidateAttributeRequest, resp *tfsdk.ValidateAttributeResponse) {
	if req.AttributeConfig == nil {
		return
	}

	value, ok := req.AttributeConfig.(TimeValue)
	if !ok {
		resp.Diagnostics.AddError("Time validation error", fmt.Sprintf("Attribute %v can not be converted to TimeValue", req.AttributePath.String()))
		return
	}

	if value.Null || value.Unknown {
		return
	}

	if time.Now().After(value.Value) {
		resp.Diagnostics.AddError("Time validation error", fmt.Sprintf("Attribute %v value must be in the future", req.AttributePath.String()))
	}
}

// VersionValidator validates that a resource version is in the specified range
type VersionValidator struct {
	Min int
	Max int
}

// UseVersionBetween creates VersionValidator
func UseVersionBetween(min, max int) tfsdk.AttributeValidator {
	return VersionValidator{min, max}
}

// Description returns validator description
func (v VersionValidator) Description(_ context.Context) string {
	return fmt.Sprintf("Checks that version string is between %v..%v", v.Min, v.Max)
}

// MarkdownDescription returns validator markdown description
func (v VersionValidator) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("Checks that version string is between %v..%v", v.Min, v.Max)
}

// Validate performs the validation.
func (v VersionValidator) Validate(_ context.Context, req tfsdk.ValidateAttributeRequest, resp *tfsdk.ValidateAttributeResponse) {
	if req.AttributeConfig == nil {
		return
	}

	value, ok := req.AttributeConfig.(types.String)
	if !ok {
		resp.Diagnostics.AddError("Version validation error", fmt.Sprintf("Attribute %v can not be converted to StringValue", req.AttributePath.String()))
		return
	}

	if value.Null || value.Unknown {
		return
	}

	var version int
	fmt.Sscan(value.Value[1:], &version) // strip leading v<xx>

	if version == 0 {
		resp.Diagnostics.AddError("Version validation error", fmt.Sprintf("Attribute %v (%v) is not a vaild version (vXX)", req.AttributePath.String(), value.Value))
		return
	}

	if version < v.Min || version > v.Max {
		resp.Diagnostics.AddError("Version validation error", fmt.Sprintf("Version %v (%v) is not in %v..%v", version, req.AttributePath.String(), v.Min, v.Max))
		return
	}
}

// MapKeysPresentValidator validates that a map has the specified keys
type MapKeysPresentValidator struct {
	Keys []string
}

// UseKeysPresentValidator creates MapKeysPresentValidator
func UseMapKeysPresentValidator(keys ...string) tfsdk.AttributeValidator {
	return MapKeysPresentValidator{Keys: keys}
}

// Description returns validator description
func (v MapKeysPresentValidator) Description(_ context.Context) string {
	return fmt.Sprintf("Checks that a map has %v keys set", v.Keys)
}

// MarkdownDescription returns validator markdown description
func (v MapKeysPresentValidator) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("Checks that a map has %v keys set", v.Keys)
}

// Validate performs the validation.
func (v MapKeysPresentValidator) Validate(_ context.Context, req tfsdk.ValidateAttributeRequest, resp *tfsdk.ValidateAttributeResponse) {
	if req.AttributeConfig == nil {
		return
	}

	value, ok := req.AttributeConfig.(types.Map)
	if !ok {
		resp.Diagnostics.AddError("Map keys validation error", fmt.Sprintf("Attribute %v can not be converted to Map", req.AttributePath.String()))
		return
	}

	if value.Null || value.Unknown {
		return
	}

OUTER:
	for _, k := range v.Keys {
		for e, _ := range value.Elems {
			if e == k {
				break OUTER
			}
		}

		resp.Diagnostics.AddError("Map keys validation error", fmt.Sprintf("Key %v must be present in the map %v", k, req.AttributePath.String()))
	}

}
