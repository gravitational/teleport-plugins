package tfschema

import (
	"context"
	fmt "fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	tftypes "github.com/hashicorp/terraform-plugin-go/tftypes"
)

// AttributeAliasPlanModifier AttributePlanModifier which copies a value to this attribute
type AttributeAliasPlanModifier struct {
	Path *tftypes.AttributePath
}

// UseMetadataNameValue creates AttributeAliasPlanModifier which takes value from metadata.name
func UseMetadataNameValue() tfsdk.AttributePlanModifier {
	return AttributeAliasPlanModifier{
		Path: tftypes.NewAttributePath().WithAttributeName("metadata").WithAttributeName("name"),
	}
}

// Description returns description
func (i AttributeAliasPlanModifier) Description(ctx context.Context) string {
	return fmt.Sprintf("Sets attribute value equal to %v", i.Path.String())
}

// MarkdownDescription returns markdown description
func (i AttributeAliasPlanModifier) MarkdownDescription(ctx context.Context) string {
	return fmt.Sprintf("Sets attribute value equal to %v", i.Path.String())
}

// Modify performs plan modification if required
func (i AttributeAliasPlanModifier) Modify(ctx context.Context, req tfsdk.ModifyAttributePlanRequest, resp *tfsdk.ModifyAttributePlanResponse) {
	if resp.AttributePlan == nil {
		return
	}

	var val attr.Value
	diags := req.Plan.GetAttribute(ctx, i.Path, &val)

	if diags.HasError() {
		return
	}

	resp.AttributePlan = val
}
