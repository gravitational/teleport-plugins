package tfschema

import (
	"context"
	fmt "fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type TimeValueInFutureValidator struct{}

func MustTimeBeInFuture() tfsdk.AttributeValidator {
	return TimeValueInFutureValidator{}
}

func (v TimeValueInFutureValidator) Description(_ context.Context) string {
	return "Checks that a time value is in future"
}

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

type VersionValidator struct {
	Min int
	Max int
}

// func MustTimeBeInFuture() tfsdk.AttributeValidator {
// 	return TimeValueInFutureValidator{}
// }

func (v VersionValidator) Description(_ context.Context) string {
	return fmt.Sprintf("Checks that version string is between %v..%v", v.Min, v.Max)
}

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
	// strip v<xx>
	fmt.Sscan(value.Value[1:], &version)

	if version == 0 {
		resp.Diagnostics.AddError("Version validation error", fmt.Sprintf("Attribute %v (%v) is not a vaild version (vXX)", req.AttributePath.String(), value.Value))
		return
	}

	if version < v.Min || version > v.Max {
		resp.Diagnostics.AddError("Version validation error", fmt.Sprintf("Version %v (%v) is not in %v..%v", version, req.AttributePath.String(), v.Min, v.Max))
		return
	}
}
