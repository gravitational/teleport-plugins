/*
Copyright 2015-2022 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: teleport/legacy/types/device.proto

package v1

import (
	context "context"
	fmt "fmt"
	math "math"
	time "time"

	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
	github_com_gravitational_teleport_plugins_terraform_tfschema "github.com/gravitational/teleport-plugins/terraform/tfschema"
	github_com_gravitational_teleport_api_types "github.com/gravitational/teleport/api/types"
	github_com_hashicorp_terraform_plugin_framework_attr "github.com/hashicorp/terraform-plugin-framework/attr"
	github_com_hashicorp_terraform_plugin_framework_diag "github.com/hashicorp/terraform-plugin-framework/diag"
	github_com_hashicorp_terraform_plugin_framework_tfsdk "github.com/hashicorp/terraform-plugin-framework/tfsdk"
	github_com_hashicorp_terraform_plugin_framework_types "github.com/hashicorp/terraform-plugin-framework/types"
	github_com_hashicorp_terraform_plugin_go_tftypes "github.com/hashicorp/terraform-plugin-go/tftypes"
	_ "google.golang.org/protobuf/types/known/timestamppb"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf
var _ = time.Kitchen

// GenSchemaDeviceV1 returns tfsdk.Schema definition for DeviceV1
func GenSchemaDeviceV1(ctx context.Context) (github_com_hashicorp_terraform_plugin_framework_tfsdk.Schema, github_com_hashicorp_terraform_plugin_framework_diag.Diagnostics) {
	return github_com_hashicorp_terraform_plugin_framework_tfsdk.Schema{Attributes: map[string]github_com_hashicorp_terraform_plugin_framework_tfsdk.Attribute{
		"Metadata.name": {
			Computed:      true,
			Optional:      false,
			PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.UseStateForUnknown()},
			Required:      false,
			Type:          github_com_hashicorp_terraform_plugin_framework_types.StringType,
		},
		"id": {
			Computed:      true,
			Optional:      false,
			PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.UseStateForUnknown()},
			Required:      false,
			Type:          github_com_hashicorp_terraform_plugin_framework_types.StringType,
		},
		"spec": {
			Attributes: github_com_hashicorp_terraform_plugin_framework_tfsdk.SingleNestedAttributes(map[string]github_com_hashicorp_terraform_plugin_framework_tfsdk.Attribute{
				"asset_tag": {
					Description: "",
					Optional:    true,
					Type:        github_com_hashicorp_terraform_plugin_framework_types.StringType,
				},
				"collected_data": {
					Attributes: github_com_hashicorp_terraform_plugin_framework_tfsdk.ListNestedAttributes(map[string]github_com_hashicorp_terraform_plugin_framework_tfsdk.Attribute{
						"collect_time": {
							Description: "",
							Optional:    true,
							Type:        github_com_gravitational_teleport_plugins_terraform_tfschema.UseRFC3339Time(),
						},
						"os_type": {
							Description: "",
							Optional:    true,
							Type:        github_com_hashicorp_terraform_plugin_framework_types.StringType,
						},
						"record_time": {
							Description: "",
							Optional:    true,
							Type:        github_com_gravitational_teleport_plugins_terraform_tfschema.UseRFC3339Time(),
						},
						"serial_number": {
							Description: "",
							Optional:    true,
							Type:        github_com_hashicorp_terraform_plugin_framework_types.StringType,
						},
					}),
					Computed:      true,
					Description:   "",
					Optional:      true,
					PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.UseStateForUnknown()},
				},
				"create_time": {
					Computed:      true,
					Description:   "",
					Optional:      true,
					PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.UseStateForUnknown()},
					Type:          github_com_gravitational_teleport_plugins_terraform_tfschema.UseRFC3339Time(),
				},
				"credential": {
					Attributes: github_com_hashicorp_terraform_plugin_framework_tfsdk.SingleNestedAttributes(map[string]github_com_hashicorp_terraform_plugin_framework_tfsdk.Attribute{
						"id": {
							Description: "",
							Optional:    true,
							Type:        github_com_hashicorp_terraform_plugin_framework_types.StringType,
						},
						"public_key_der": {
							Description: "",
							Optional:    true,
							Type:        github_com_hashicorp_terraform_plugin_framework_types.StringType,
						},
					}),
					Description: "",
					Optional:    true,
				},
				"enroll_status": {
					Description: "",
					Optional:    true,
					Type:        github_com_hashicorp_terraform_plugin_framework_types.StringType,
				},
				"os_type": {
					Description: "",
					Required:    true,
					Type:        github_com_hashicorp_terraform_plugin_framework_types.StringType,
				},
				"update_time": {
					Computed:      true,
					Description:   "",
					Optional:      true,
					PlanModifiers: []github_com_hashicorp_terraform_plugin_framework_tfsdk.AttributePlanModifier{github_com_hashicorp_terraform_plugin_framework_tfsdk.UseStateForUnknown()},
					Type:          github_com_gravitational_teleport_plugins_terraform_tfschema.UseRFC3339Time(),
				},
			}),
			Description: "Specification of the device.",
			Optional:    true,
		},
	}}, nil
}

// CopyDeviceV1FromTerraform copies contents of the source Terraform object into a target struct
func CopyDeviceV1FromTerraform(_ context.Context, tf github_com_hashicorp_terraform_plugin_framework_types.Object, obj *github_com_gravitational_teleport_api_types.DeviceV1) github_com_hashicorp_terraform_plugin_framework_diag.Diagnostics {
	var diags github_com_hashicorp_terraform_plugin_framework_diag.Diagnostics
	{
		a, ok := tf.Attrs["spec"]
		if !ok {
			diags.Append(attrReadMissingDiag{"DeviceV1.spec"})
		} else {
			v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.Object)
			if !ok {
				diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec", "github.com/hashicorp/terraform-plugin-framework/types.Object"})
			} else {
				obj.Spec = nil
				if !v.Null && !v.Unknown {
					tf := v
					obj.Spec = &github_com_gravitational_teleport_api_types.DeviceSpec{}
					obj := obj.Spec
					{
						a, ok := tf.Attrs["os_type"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.spec.os_type"})
						} else {
							v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.os_type", "github.com/hashicorp/terraform-plugin-framework/types.String"})
							} else {
								var t string
								if !v.Null && !v.Unknown {
									t = string(v.Value)
								}
								obj.OsType = t
							}
						}
					}
					{
						a, ok := tf.Attrs["asset_tag"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.spec.asset_tag"})
						} else {
							v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.asset_tag", "github.com/hashicorp/terraform-plugin-framework/types.String"})
							} else {
								var t string
								if !v.Null && !v.Unknown {
									t = string(v.Value)
								}
								obj.AssetTag = t
							}
						}
					}
					{
						a, ok := tf.Attrs["create_time"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.spec.create_time"})
						} else {
							v, ok := a.(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.create_time", "github.com/gravitational/teleport-plugins/terraform/tfschema.TimeValue"})
							} else {
								var t *time.Time
								if !v.Null && !v.Unknown {
									c := time.Time(v.Value)
									t = &c
								}
								obj.CreateTime = t
							}
						}
					}
					{
						a, ok := tf.Attrs["update_time"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.spec.update_time"})
						} else {
							v, ok := a.(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.update_time", "github.com/gravitational/teleport-plugins/terraform/tfschema.TimeValue"})
							} else {
								var t *time.Time
								if !v.Null && !v.Unknown {
									c := time.Time(v.Value)
									t = &c
								}
								obj.UpdateTime = t
							}
						}
					}
					{
						a, ok := tf.Attrs["enroll_status"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.spec.enroll_status"})
						} else {
							v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.enroll_status", "github.com/hashicorp/terraform-plugin-framework/types.String"})
							} else {
								var t string
								if !v.Null && !v.Unknown {
									t = string(v.Value)
								}
								obj.EnrollStatus = t
							}
						}
					}
					{
						a, ok := tf.Attrs["credential"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.spec.credential"})
						} else {
							v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.Object)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.credential", "github.com/hashicorp/terraform-plugin-framework/types.Object"})
							} else {
								obj.Credential = nil
								if !v.Null && !v.Unknown {
									tf := v
									obj.Credential = &github_com_gravitational_teleport_api_types.DeviceCredential{}
									obj := obj.Credential
									{
										a, ok := tf.Attrs["id"]
										if !ok {
											diags.Append(attrReadMissingDiag{"DeviceV1.spec.credential.id"})
										} else {
											v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
											if !ok {
												diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.credential.id", "github.com/hashicorp/terraform-plugin-framework/types.String"})
											} else {
												var t string
												if !v.Null && !v.Unknown {
													t = string(v.Value)
												}
												obj.Id = t
											}
										}
									}
									{
										a, ok := tf.Attrs["public_key_der"]
										if !ok {
											diags.Append(attrReadMissingDiag{"DeviceV1.spec.credential.public_key_der"})
										} else {
											v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
											if !ok {
												diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.credential.public_key_der", "github.com/hashicorp/terraform-plugin-framework/types.String"})
											} else {
												var t []byte
												if !v.Null && !v.Unknown {
													t = []byte(v.Value)
												}
												obj.PublicKeyDer = t
											}
										}
									}
								}
							}
						}
					}
					{
						a, ok := tf.Attrs["collected_data"]
						if !ok {
							diags.Append(attrReadMissingDiag{"DeviceV1.spec.collected_data"})
						} else {
							v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.List)
							if !ok {
								diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.collected_data", "github.com/hashicorp/terraform-plugin-framework/types.List"})
							} else {
								obj.CollectedData = make([]*github_com_gravitational_teleport_api_types.DeviceCollectedData, len(v.Elems))
								if !v.Null && !v.Unknown {
									for k, a := range v.Elems {
										v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.Object)
										if !ok {
											diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.collected_data", "github_com_hashicorp_terraform_plugin_framework_types.Object"})
										} else {
											var t *github_com_gravitational_teleport_api_types.DeviceCollectedData
											if !v.Null && !v.Unknown {
												tf := v
												t = &github_com_gravitational_teleport_api_types.DeviceCollectedData{}
												obj := t
												{
													a, ok := tf.Attrs["collect_time"]
													if !ok {
														diags.Append(attrReadMissingDiag{"DeviceV1.spec.collected_data.collect_time"})
													} else {
														v, ok := a.(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
														if !ok {
															diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.collected_data.collect_time", "github.com/gravitational/teleport-plugins/terraform/tfschema.TimeValue"})
														} else {
															var t *time.Time
															if !v.Null && !v.Unknown {
																c := time.Time(v.Value)
																t = &c
															}
															obj.CollectTime = t
														}
													}
												}
												{
													a, ok := tf.Attrs["record_time"]
													if !ok {
														diags.Append(attrReadMissingDiag{"DeviceV1.spec.collected_data.record_time"})
													} else {
														v, ok := a.(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
														if !ok {
															diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.collected_data.record_time", "github.com/gravitational/teleport-plugins/terraform/tfschema.TimeValue"})
														} else {
															var t *time.Time
															if !v.Null && !v.Unknown {
																c := time.Time(v.Value)
																t = &c
															}
															obj.RecordTime = t
														}
													}
												}
												{
													a, ok := tf.Attrs["os_type"]
													if !ok {
														diags.Append(attrReadMissingDiag{"DeviceV1.spec.collected_data.os_type"})
													} else {
														v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
														if !ok {
															diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.collected_data.os_type", "github.com/hashicorp/terraform-plugin-framework/types.String"})
														} else {
															var t string
															if !v.Null && !v.Unknown {
																t = string(v.Value)
															}
															obj.OsType = t
														}
													}
												}
												{
													a, ok := tf.Attrs["serial_number"]
													if !ok {
														diags.Append(attrReadMissingDiag{"DeviceV1.spec.collected_data.serial_number"})
													} else {
														v, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.String)
														if !ok {
															diags.Append(attrReadConversionFailureDiag{"DeviceV1.spec.collected_data.serial_number", "github.com/hashicorp/terraform-plugin-framework/types.String"})
														} else {
															var t string
															if !v.Null && !v.Unknown {
																t = string(v.Value)
															}
															obj.SerialNumber = t
														}
													}
												}
											}
											obj.CollectedData[k] = t
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return diags
}

// CopyDeviceV1ToTerraform copies contents of the source Terraform object into a target struct
func CopyDeviceV1ToTerraform(ctx context.Context, obj github_com_gravitational_teleport_api_types.DeviceV1, tf *github_com_hashicorp_terraform_plugin_framework_types.Object) github_com_hashicorp_terraform_plugin_framework_diag.Diagnostics {
	var diags github_com_hashicorp_terraform_plugin_framework_diag.Diagnostics
	tf.Null = false
	tf.Unknown = false
	if tf.Attrs == nil {
		tf.Attrs = make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value)
	}
	{
		a, ok := tf.AttrTypes["spec"]
		if !ok {
			diags.Append(attrWriteMissingDiag{"DeviceV1.spec"})
		} else {
			o, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.ObjectType)
			if !ok {
				diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec", "github.com/hashicorp/terraform-plugin-framework/types.ObjectType"})
			} else {
				v, ok := tf.Attrs["spec"].(github_com_hashicorp_terraform_plugin_framework_types.Object)
				if !ok {
					v = github_com_hashicorp_terraform_plugin_framework_types.Object{

						AttrTypes: o.AttrTypes,
						Attrs:     make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(o.AttrTypes)),
					}
				} else {
					if v.Attrs == nil {
						v.Attrs = make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(tf.AttrTypes))
					}
				}
				if obj.Spec == nil {
					v.Null = true
				} else {
					obj := obj.Spec
					tf := &v
					{
						t, ok := tf.AttrTypes["os_type"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.spec.os_type"})
						} else {
							v, ok := tf.Attrs["os_type"].(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
								if err != nil {
									diags.Append(attrWriteGeneralError{"DeviceV1.spec.os_type", err})
								}
								v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
								if !ok {
									diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.os_type", "github.com/hashicorp/terraform-plugin-framework/types.String"})
								}
								v.Null = string(obj.OsType) == ""
							}
							v.Value = string(obj.OsType)
							v.Unknown = false
							tf.Attrs["os_type"] = v
						}
					}
					{
						t, ok := tf.AttrTypes["asset_tag"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.spec.asset_tag"})
						} else {
							v, ok := tf.Attrs["asset_tag"].(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
								if err != nil {
									diags.Append(attrWriteGeneralError{"DeviceV1.spec.asset_tag", err})
								}
								v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
								if !ok {
									diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.asset_tag", "github.com/hashicorp/terraform-plugin-framework/types.String"})
								}
								v.Null = string(obj.AssetTag) == ""
							}
							v.Value = string(obj.AssetTag)
							v.Unknown = false
							tf.Attrs["asset_tag"] = v
						}
					}
					{
						t, ok := tf.AttrTypes["create_time"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.spec.create_time"})
						} else {
							v, ok := tf.Attrs["create_time"].(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
							if !ok {
								i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
								if err != nil {
									diags.Append(attrWriteGeneralError{"DeviceV1.spec.create_time", err})
								}
								v, ok = i.(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
								if !ok {
									diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.create_time", "github.com/gravitational/teleport-plugins/terraform/tfschema.TimeValue"})
								}
								v.Null = false
							}
							if obj.CreateTime == nil {
								v.Null = true
							} else {
								v.Null = false
								v.Value = time.Time(*obj.CreateTime)
							}
							v.Unknown = false
							tf.Attrs["create_time"] = v
						}
					}
					{
						t, ok := tf.AttrTypes["update_time"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.spec.update_time"})
						} else {
							v, ok := tf.Attrs["update_time"].(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
							if !ok {
								i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
								if err != nil {
									diags.Append(attrWriteGeneralError{"DeviceV1.spec.update_time", err})
								}
								v, ok = i.(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
								if !ok {
									diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.update_time", "github.com/gravitational/teleport-plugins/terraform/tfschema.TimeValue"})
								}
								v.Null = false
							}
							if obj.UpdateTime == nil {
								v.Null = true
							} else {
								v.Null = false
								v.Value = time.Time(*obj.UpdateTime)
							}
							v.Unknown = false
							tf.Attrs["update_time"] = v
						}
					}
					{
						t, ok := tf.AttrTypes["enroll_status"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.spec.enroll_status"})
						} else {
							v, ok := tf.Attrs["enroll_status"].(github_com_hashicorp_terraform_plugin_framework_types.String)
							if !ok {
								i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
								if err != nil {
									diags.Append(attrWriteGeneralError{"DeviceV1.spec.enroll_status", err})
								}
								v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
								if !ok {
									diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.enroll_status", "github.com/hashicorp/terraform-plugin-framework/types.String"})
								}
								v.Null = string(obj.EnrollStatus) == ""
							}
							v.Value = string(obj.EnrollStatus)
							v.Unknown = false
							tf.Attrs["enroll_status"] = v
						}
					}
					{
						a, ok := tf.AttrTypes["credential"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.spec.credential"})
						} else {
							o, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.ObjectType)
							if !ok {
								diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.credential", "github.com/hashicorp/terraform-plugin-framework/types.ObjectType"})
							} else {
								v, ok := tf.Attrs["credential"].(github_com_hashicorp_terraform_plugin_framework_types.Object)
								if !ok {
									v = github_com_hashicorp_terraform_plugin_framework_types.Object{

										AttrTypes: o.AttrTypes,
										Attrs:     make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(o.AttrTypes)),
									}
								} else {
									if v.Attrs == nil {
										v.Attrs = make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(tf.AttrTypes))
									}
								}
								if obj.Credential == nil {
									v.Null = true
								} else {
									obj := obj.Credential
									tf := &v
									{
										t, ok := tf.AttrTypes["id"]
										if !ok {
											diags.Append(attrWriteMissingDiag{"DeviceV1.spec.credential.id"})
										} else {
											v, ok := tf.Attrs["id"].(github_com_hashicorp_terraform_plugin_framework_types.String)
											if !ok {
												i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
												if err != nil {
													diags.Append(attrWriteGeneralError{"DeviceV1.spec.credential.id", err})
												}
												v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
												if !ok {
													diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.credential.id", "github.com/hashicorp/terraform-plugin-framework/types.String"})
												}
												v.Null = string(obj.Id) == ""
											}
											v.Value = string(obj.Id)
											v.Unknown = false
											tf.Attrs["id"] = v
										}
									}
									{
										t, ok := tf.AttrTypes["public_key_der"]
										if !ok {
											diags.Append(attrWriteMissingDiag{"DeviceV1.spec.credential.public_key_der"})
										} else {
											v, ok := tf.Attrs["public_key_der"].(github_com_hashicorp_terraform_plugin_framework_types.String)
											if !ok {
												i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
												if err != nil {
													diags.Append(attrWriteGeneralError{"DeviceV1.spec.credential.public_key_der", err})
												}
												v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
												if !ok {
													diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.credential.public_key_der", "github.com/hashicorp/terraform-plugin-framework/types.String"})
												}
												v.Null = string(obj.PublicKeyDer) == ""
											}
											v.Value = string(obj.PublicKeyDer)
											v.Unknown = false
											tf.Attrs["public_key_der"] = v
										}
									}
								}
								v.Unknown = false
								tf.Attrs["credential"] = v
							}
						}
					}
					{
						a, ok := tf.AttrTypes["collected_data"]
						if !ok {
							diags.Append(attrWriteMissingDiag{"DeviceV1.spec.collected_data"})
						} else {
							o, ok := a.(github_com_hashicorp_terraform_plugin_framework_types.ListType)
							if !ok {
								diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.collected_data", "github.com/hashicorp/terraform-plugin-framework/types.ListType"})
							} else {
								c, ok := tf.Attrs["collected_data"].(github_com_hashicorp_terraform_plugin_framework_types.List)
								if !ok {
									c = github_com_hashicorp_terraform_plugin_framework_types.List{

										ElemType: o.ElemType,
										Elems:    make([]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(obj.CollectedData)),
										Null:     true,
									}
								} else {
									if c.Elems == nil {
										c.Elems = make([]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(obj.CollectedData))
									}
								}
								if obj.CollectedData != nil {
									o := o.ElemType.(github_com_hashicorp_terraform_plugin_framework_types.ObjectType)
									if len(obj.CollectedData) != len(c.Elems) {
										c.Elems = make([]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(obj.CollectedData))
									}
									for k, a := range obj.CollectedData {
										v, ok := tf.Attrs["collected_data"].(github_com_hashicorp_terraform_plugin_framework_types.Object)
										if !ok {
											v = github_com_hashicorp_terraform_plugin_framework_types.Object{

												AttrTypes: o.AttrTypes,
												Attrs:     make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(o.AttrTypes)),
											}
										} else {
											if v.Attrs == nil {
												v.Attrs = make(map[string]github_com_hashicorp_terraform_plugin_framework_attr.Value, len(tf.AttrTypes))
											}
										}
										if a == nil {
											v.Null = true
										} else {
											obj := a
											tf := &v
											{
												t, ok := tf.AttrTypes["collect_time"]
												if !ok {
													diags.Append(attrWriteMissingDiag{"DeviceV1.spec.collected_data.collect_time"})
												} else {
													v, ok := tf.Attrs["collect_time"].(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
													if !ok {
														i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
														if err != nil {
															diags.Append(attrWriteGeneralError{"DeviceV1.spec.collected_data.collect_time", err})
														}
														v, ok = i.(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
														if !ok {
															diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.collected_data.collect_time", "github.com/gravitational/teleport-plugins/terraform/tfschema.TimeValue"})
														}
														v.Null = false
													}
													if obj.CollectTime == nil {
														v.Null = true
													} else {
														v.Null = false
														v.Value = time.Time(*obj.CollectTime)
													}
													v.Unknown = false
													tf.Attrs["collect_time"] = v
												}
											}
											{
												t, ok := tf.AttrTypes["record_time"]
												if !ok {
													diags.Append(attrWriteMissingDiag{"DeviceV1.spec.collected_data.record_time"})
												} else {
													v, ok := tf.Attrs["record_time"].(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
													if !ok {
														i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
														if err != nil {
															diags.Append(attrWriteGeneralError{"DeviceV1.spec.collected_data.record_time", err})
														}
														v, ok = i.(github_com_gravitational_teleport_plugins_terraform_tfschema.TimeValue)
														if !ok {
															diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.collected_data.record_time", "github.com/gravitational/teleport-plugins/terraform/tfschema.TimeValue"})
														}
														v.Null = false
													}
													if obj.RecordTime == nil {
														v.Null = true
													} else {
														v.Null = false
														v.Value = time.Time(*obj.RecordTime)
													}
													v.Unknown = false
													tf.Attrs["record_time"] = v
												}
											}
											{
												t, ok := tf.AttrTypes["os_type"]
												if !ok {
													diags.Append(attrWriteMissingDiag{"DeviceV1.spec.collected_data.os_type"})
												} else {
													v, ok := tf.Attrs["os_type"].(github_com_hashicorp_terraform_plugin_framework_types.String)
													if !ok {
														i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
														if err != nil {
															diags.Append(attrWriteGeneralError{"DeviceV1.spec.collected_data.os_type", err})
														}
														v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
														if !ok {
															diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.collected_data.os_type", "github.com/hashicorp/terraform-plugin-framework/types.String"})
														}
														v.Null = string(obj.OsType) == ""
													}
													v.Value = string(obj.OsType)
													v.Unknown = false
													tf.Attrs["os_type"] = v
												}
											}
											{
												t, ok := tf.AttrTypes["serial_number"]
												if !ok {
													diags.Append(attrWriteMissingDiag{"DeviceV1.spec.collected_data.serial_number"})
												} else {
													v, ok := tf.Attrs["serial_number"].(github_com_hashicorp_terraform_plugin_framework_types.String)
													if !ok {
														i, err := t.ValueFromTerraform(ctx, github_com_hashicorp_terraform_plugin_go_tftypes.NewValue(t.TerraformType(ctx), nil))
														if err != nil {
															diags.Append(attrWriteGeneralError{"DeviceV1.spec.collected_data.serial_number", err})
														}
														v, ok = i.(github_com_hashicorp_terraform_plugin_framework_types.String)
														if !ok {
															diags.Append(attrWriteConversionFailureDiag{"DeviceV1.spec.collected_data.serial_number", "github.com/hashicorp/terraform-plugin-framework/types.String"})
														}
														v.Null = string(obj.SerialNumber) == ""
													}
													v.Value = string(obj.SerialNumber)
													v.Unknown = false
													tf.Attrs["serial_number"] = v
												}
											}
										}
										v.Unknown = false
										c.Elems[k] = v
									}
									if len(obj.CollectedData) > 0 {
										c.Null = false
									}
								}
								c.Unknown = false
								tf.Attrs["collected_data"] = c
							}
						}
					}
				}
				v.Unknown = false
				tf.Attrs["spec"] = v
			}
		}
	}
	return diags
}

// attrReadMissingDiag represents diagnostic message on an attribute missing in the source object
type attrReadMissingDiag struct {
	Path string
}

func (d attrReadMissingDiag) Severity() github_com_hashicorp_terraform_plugin_framework_diag.Severity {
	return github_com_hashicorp_terraform_plugin_framework_diag.SeverityError
}

func (d attrReadMissingDiag) Summary() string {
	return "Error reading from Terraform object"
}

func (d attrReadMissingDiag) Detail() string {
	return fmt.Sprintf("A value for %v is missing in the source Terraform object Attrs", d.Path)
}

func (d attrReadMissingDiag) Equal(o github_com_hashicorp_terraform_plugin_framework_diag.Diagnostic) bool {
	return (d.Severity() == o.Severity()) && (d.Summary() == o.Summary()) && (d.Detail() == o.Detail())
}

// attrReadConversionFailureDiag represents diagnostic message on a failed type conversion on read
type attrReadConversionFailureDiag struct {
	Path string
	Type string
}

func (d attrReadConversionFailureDiag) Severity() github_com_hashicorp_terraform_plugin_framework_diag.Severity {
	return github_com_hashicorp_terraform_plugin_framework_diag.SeverityError
}

func (d attrReadConversionFailureDiag) Summary() string {
	return "Error reading from Terraform object"
}

func (d attrReadConversionFailureDiag) Detail() string {
	return fmt.Sprintf("A value for %v can not be converted to %v", d.Path, d.Type)
}

func (d attrReadConversionFailureDiag) Equal(o github_com_hashicorp_terraform_plugin_framework_diag.Diagnostic) bool {
	return (d.Severity() == o.Severity()) && (d.Summary() == o.Summary()) && (d.Detail() == o.Detail())
}

// attrWriteMissingDiag represents diagnostic message on an attribute missing in the target object
type attrWriteMissingDiag struct {
	Path string
}

func (d attrWriteMissingDiag) Severity() github_com_hashicorp_terraform_plugin_framework_diag.Severity {
	return github_com_hashicorp_terraform_plugin_framework_diag.SeverityError
}

func (d attrWriteMissingDiag) Summary() string {
	return "Error writing to Terraform object"
}

func (d attrWriteMissingDiag) Detail() string {
	return fmt.Sprintf("A value for %v is missing in the source Terraform object AttrTypes", d.Path)
}

func (d attrWriteMissingDiag) Equal(o github_com_hashicorp_terraform_plugin_framework_diag.Diagnostic) bool {
	return (d.Severity() == o.Severity()) && (d.Summary() == o.Summary()) && (d.Detail() == o.Detail())
}

// attrWriteConversionFailureDiag represents diagnostic message on a failed type conversion on write
type attrWriteConversionFailureDiag struct {
	Path string
	Type string
}

func (d attrWriteConversionFailureDiag) Severity() github_com_hashicorp_terraform_plugin_framework_diag.Severity {
	return github_com_hashicorp_terraform_plugin_framework_diag.SeverityError
}

func (d attrWriteConversionFailureDiag) Summary() string {
	return "Error writing to Terraform object"
}

func (d attrWriteConversionFailureDiag) Detail() string {
	return fmt.Sprintf("A value for %v can not be converted to %v", d.Path, d.Type)
}

func (d attrWriteConversionFailureDiag) Equal(o github_com_hashicorp_terraform_plugin_framework_diag.Diagnostic) bool {
	return (d.Severity() == o.Severity()) && (d.Summary() == o.Summary()) && (d.Detail() == o.Detail())
}

// attrWriteGeneralError represents diagnostic message on a generic error on write
type attrWriteGeneralError struct {
	Path string
	Err  error
}

func (d attrWriteGeneralError) Severity() github_com_hashicorp_terraform_plugin_framework_diag.Severity {
	return github_com_hashicorp_terraform_plugin_framework_diag.SeverityError
}

func (d attrWriteGeneralError) Summary() string {
	return "Error writing to Terraform object"
}

func (d attrWriteGeneralError) Detail() string {
	return fmt.Sprintf("%s: %s", d.Path, d.Err.Error())
}

func (d attrWriteGeneralError) Equal(o github_com_hashicorp_terraform_plugin_framework_diag.Diagnostic) bool {
	return (d.Severity() == o.Severity()) && (d.Summary() == o.Summary()) && (d.Detail() == o.Detail())
}