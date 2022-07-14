package tfschema

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type boolDefaultModifier struct {
	Default bool
}

// Description returns a plain text description of the validator's behavior, suitable for a practitioner to understand its impact.
func (m boolDefaultModifier) Description(ctx context.Context) string {
	return "Sets value always to null"
}

// MarkdownDescription returns a markdown formatted description of the validator's behavior, suitable for a practitioner to understand its impact.
func (m boolDefaultModifier) MarkdownDescription(ctx context.Context) string {
	return "Sets value always to null"
}

// Modify runs the logic of the plan modifier: it sets default value for any unknonwn
func (m boolDefaultModifier) Modify(ctx context.Context, req tfsdk.ModifyAttributePlanRequest, resp *tfsdk.ModifyAttributePlanResponse) {
	var b types.Bool
	diags := tfsdk.ValueAs(ctx, req.AttributePlan, &b)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	if b.Unknown {
		resp.AttributePlan = types.Bool{Value: m.Default}
	}
}

func boolDefault(def bool) boolDefaultModifier {
	return boolDefaultModifier{Default: def}
}
