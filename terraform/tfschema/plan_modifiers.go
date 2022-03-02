package tfschema

import (
	"context"
	fmt "fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// AttributeConstPlanModifier AttributePlanModifier which sets an attribute to the constant value (used for singular resources id)
type AttributeConstPlanModifier struct {
	Value attr.Value
}

// UseConstValue creates AttributeAliasPlanModifier which takes value from a const
func UseConstStringValue(value string) tfsdk.AttributePlanModifier {
	return AttributeConstPlanModifier{
		Value: types.String{Value: value},
	}
}

// Description returns description
func (i AttributeConstPlanModifier) Description(ctx context.Context) string {
	return fmt.Sprintf("Sets attribute value equal to %v", i.Value)
}

// MarkdownDescription returns markdown description
func (i AttributeConstPlanModifier) MarkdownDescription(ctx context.Context) string {
	return fmt.Sprintf("Sets attribute value equal to %v", i.Value)
}

// Modify performs plan modification if required
func (i AttributeConstPlanModifier) Modify(ctx context.Context, req tfsdk.ModifyAttributePlanRequest, resp *tfsdk.ModifyAttributePlanResponse) {
	if resp.AttributePlan == nil {
		return
	}

	resp.AttributePlan = i.Value
}
