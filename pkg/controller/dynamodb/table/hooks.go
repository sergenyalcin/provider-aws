/*
Copyright 2020 The Crossplane Authors.

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

package table

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	svcsdk "github.com/aws/aws-sdk-go/service/dynamodb"
	svcsdkapi "github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	svcapitypes "github.com/crossplane/provider-aws/apis/dynamodb/v1alpha1"
	aws "github.com/crossplane/provider-aws/pkg/clients"
)

// SetupTable adds a controller that reconciles Table.
func SetupTable(mgr ctrl.Manager, l logging.Logger, rl workqueue.RateLimiter, poll time.Duration) error {
	name := managed.ControllerName(svcapitypes.TableGroupKind)
	opts := []option{
		func(e *external) {
			e.preObserve = preObserve
			e.postObserve = postObserve
			e.preCreate = preCreate
			e.preDelete = preDelete
			e.lateInitialize = lateInitialize
			e.isUpToDate = isUpToDate
			u := &updateClient{client: e.client}
			e.preUpdate = u.preUpdate
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(controller.Options{
			RateLimiter: ratelimiter.NewController(rl),
		}).
		For(&svcapitypes.Table{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(svcapitypes.TableGroupVersionKind),
			managed.WithExternalConnecter(&connector{kube: mgr.GetClient(), opts: opts}),
			managed.WithInitializers(
				managed.NewNameAsExternalName(mgr.GetClient()),
				managed.NewDefaultProviderConfig(mgr.GetClient()),
				&tagger{kube: mgr.GetClient()}),
			managed.WithPollInterval(poll),
			managed.WithLogger(l.WithValues("controller", name)),
			managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name)))))
}

func preObserve(_ context.Context, cr *svcapitypes.Table, obj *svcsdk.DescribeTableInput) error {
	obj.TableName = aws.String(meta.GetExternalName(cr))
	return nil
}
func preCreate(_ context.Context, cr *svcapitypes.Table, obj *svcsdk.CreateTableInput) error {
	obj.TableName = aws.String(meta.GetExternalName(cr))
	return nil
}
func preDelete(_ context.Context, cr *svcapitypes.Table, obj *svcsdk.DeleteTableInput) (bool, error) {
	obj.TableName = aws.String(meta.GetExternalName(cr))
	return false, nil
}

func postObserve(_ context.Context, cr *svcapitypes.Table, resp *svcsdk.DescribeTableOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	switch aws.StringValue(resp.Table.TableStatus) {
	case string(svcapitypes.TableStatus_SDK_CREATING):
		cr.SetConditions(xpv1.Creating())
	case string(svcapitypes.TableStatus_SDK_DELETING):
		cr.SetConditions(xpv1.Deleting())
	case string(svcapitypes.TableStatus_SDK_ACTIVE):
		cr.SetConditions(xpv1.Available())
	case string(svcapitypes.TableStatus_SDK_ARCHIVED), string(svcapitypes.TableStatus_SDK_INACCESSIBLE_ENCRYPTION_CREDENTIALS), string(svcapitypes.TableStatus_SDK_ARCHIVING):
		cr.SetConditions(xpv1.Unavailable())
	}

	obs.ConnectionDetails = managed.ConnectionDetails{
		"tableName":         []byte(meta.GetExternalName(cr)),
		"tableArn":          []byte(aws.StringValue(resp.Table.TableArn)),
		"latestStreamArn":   []byte(aws.StringValue(resp.Table.LatestStreamArn)),
		"latestStreamLabel": []byte(aws.StringValue(resp.Table.LatestStreamLabel)),
	}

	return obs, nil
}

type tagger struct {
	kube client.Client
}

func (e *tagger) Initialize(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*svcapitypes.Table)
	if !ok {
		return errors.New(errUnexpectedObject)
	}
	tagMap := map[string]string{}
	for _, t := range cr.Spec.ForProvider.Tags {
		tagMap[aws.StringValue(t.Key)] = aws.StringValue(t.Value)
	}
	for k, v := range resource.GetExternalTags(cr) {
		tagMap[k] = v
	}
	tags := make([]*svcapitypes.Tag, 0)
	for k, v := range tagMap {
		tags = append(tags, &svcapitypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	sort.Slice(tags, func(i, j int) bool {
		return aws.StringValue(tags[i].Key) < aws.StringValue(tags[j].Key)
	})
	if cmp.Equal(cr.Spec.ForProvider.Tags, tags) {
		return nil
	}
	cr.Spec.ForProvider.Tags = tags
	return errors.Wrap(e.kube.Update(ctx, cr), "cannot update Table Spec")
}

// NOTE(muvaf): The rest is taken from manually written controller.

func lateInitialize(in *svcapitypes.TableParameters, t *svcsdk.DescribeTableOutput) error { // nolint:gocyclo,unparam
	if t == nil {
		return nil
	}

	if len(in.AttributeDefinitions) == 0 && len(t.Table.AttributeDefinitions) != 0 {
		in.AttributeDefinitions = buildAttributeDefinitions(t.Table.AttributeDefinitions)
	}

	if len(in.GlobalSecondaryIndexes) == 0 && len(t.Table.GlobalSecondaryIndexes) != 0 {
		in.GlobalSecondaryIndexes = buildGlobalIndexes(t.Table.GlobalSecondaryIndexes)
	}

	if len(in.LocalSecondaryIndexes) == 0 && len(t.Table.LocalSecondaryIndexes) != 0 {
		in.LocalSecondaryIndexes = buildLocalIndexes(t.Table.LocalSecondaryIndexes)
	}

	if len(in.KeySchema) == 0 && len(t.Table.KeySchema) != 0 {
		in.KeySchema = buildAlphaKeyElements(t.Table.KeySchema)
	}

	if in.ProvisionedThroughput == nil && t.Table.ProvisionedThroughput != nil {
		in.ProvisionedThroughput = &svcapitypes.ProvisionedThroughput{
			ReadCapacityUnits:  t.Table.ProvisionedThroughput.ReadCapacityUnits,
			WriteCapacityUnits: t.Table.ProvisionedThroughput.WriteCapacityUnits,
		}
	}
	if t.Table.SSEDescription != nil {
		if in.SSESpecification == nil {
			in.SSESpecification = &svcapitypes.SSESpecification{}
		}
		if in.SSESpecification.Enabled == nil && t.Table.SSEDescription.Status != nil {
			in.SSESpecification.Enabled = aws.Bool(*t.Table.SSEDescription.Status == string(svcapitypes.SSEStatus_ENABLED))
		}
		if in.SSESpecification.KMSMasterKeyID == nil && t.Table.SSEDescription.KMSMasterKeyArn != nil {
			in.SSESpecification.KMSMasterKeyID = t.Table.SSEDescription.KMSMasterKeyArn
		}
		if in.SSESpecification.SSEType == nil && t.Table.SSEDescription.SSEType != nil {
			in.SSESpecification.SSEType = t.Table.SSEDescription.SSEType
		}
	}
	if in.StreamSpecification == nil && t.Table.StreamSpecification != nil {
		in.StreamSpecification = &svcapitypes.StreamSpecification{
			StreamEnabled:  t.Table.StreamSpecification.StreamEnabled,
			StreamViewType: t.Table.StreamSpecification.StreamViewType,
		}
	}

	if in.BillingMode == nil && t.Table.BillingModeSummary != nil {
		in.BillingMode = t.Table.BillingModeSummary.BillingMode
	}

	return nil
}

func buildAlphaKeyElements(keys []*svcsdk.KeySchemaElement) []*svcapitypes.KeySchemaElement {
	if len(keys) == 0 {
		return nil
	}
	keyElements := make([]*svcapitypes.KeySchemaElement, len(keys))
	for i, val := range keys {
		keyElements[i] = &svcapitypes.KeySchemaElement{
			AttributeName: val.AttributeName,
			KeyType:       val.KeyType,
		}
	}
	return keyElements
}

func buildAttributeDefinitions(attributes []*svcsdk.AttributeDefinition) []*svcapitypes.AttributeDefinition {
	if len(attributes) == 0 {
		return nil
	}
	attributeDefinitions := make([]*svcapitypes.AttributeDefinition, len(attributes))
	for i, val := range attributes {
		attributeDefinitions[i] = &svcapitypes.AttributeDefinition{
			AttributeName: val.AttributeName,
			AttributeType: val.AttributeType,
		}
	}
	return attributeDefinitions
}

func buildGlobalIndexes(indexes []*svcsdk.GlobalSecondaryIndexDescription) []*svcapitypes.GlobalSecondaryIndex {
	if len(indexes) == 0 {
		return nil
	}
	globalSecondaryIndexes := make([]*svcapitypes.GlobalSecondaryIndex, len(indexes))
	for i, val := range indexes {
		globalSecondaryIndexes[i] = &svcapitypes.GlobalSecondaryIndex{
			IndexName: val.IndexName,
			KeySchema: buildAlphaKeyElements(val.KeySchema),
		}
		if val.Projection != nil {
			globalSecondaryIndexes[i].Projection = &svcapitypes.Projection{
				NonKeyAttributes: val.Projection.NonKeyAttributes,
				ProjectionType:   val.Projection.ProjectionType,
			}
		}
	}
	return globalSecondaryIndexes
}

func buildLocalIndexes(indexes []*svcsdk.LocalSecondaryIndexDescription) []*svcapitypes.LocalSecondaryIndex {
	if len(indexes) == 0 {
		return nil
	}
	localSecondaryIndexes := make([]*svcapitypes.LocalSecondaryIndex, len(indexes))
	for i, val := range indexes {
		localSecondaryIndexes[i] = &svcapitypes.LocalSecondaryIndex{
			IndexName: val.IndexName,
			KeySchema: buildAlphaKeyElements(val.KeySchema),
		}
		if val.Projection != nil {
			localSecondaryIndexes[i].Projection = &svcapitypes.Projection{
				NonKeyAttributes: val.Projection.NonKeyAttributes,
				ProjectionType:   val.Projection.ProjectionType,
			}
		}
	}
	return localSecondaryIndexes
}

// createPatch creates a *svcapitypes.TableParameters that has only the changed
// values between the target *svcapitypes.TableParameters and the current
// *dynamodb.TableDescription
func createPatch(in *svcsdk.DescribeTableOutput, target *svcapitypes.TableParameters) (*svcapitypes.TableParameters, error) {
	currentParams := &svcapitypes.TableParameters{}
	if err := lateInitialize(currentParams, in); err != nil {
		return nil, err
	}

	jsonPatch, err := aws.CreateJSONPatch(currentParams, target)
	if err != nil {
		return nil, err
	}
	patch := &svcapitypes.TableParameters{}
	if err := json.Unmarshal(jsonPatch, patch); err != nil {
		return nil, err
	}
	return patch, nil
}

func isUpToDate(cr *svcapitypes.Table, resp *svcsdk.DescribeTableOutput) (bool, error) {
	// A table that's currently updating or creating can't be updated, so we
	// temporarily consider it to be up-to-date no matter what.
	switch aws.StringValue(cr.Status.AtProvider.TableStatus) {
	case string(svcapitypes.TableStatus_SDK_UPDATING), string(svcapitypes.TableStatus_SDK_CREATING):
		return true, nil
	}

	// Similarly, a table that's currently updating its SSE status can't be
	// updated, so we temporarily consider it to be up-to-date.
	if cr.Status.AtProvider.SSEDescription != nil && aws.StringValue(cr.Status.AtProvider.SSEDescription.Status) == string(svcapitypes.SSEStatus_UPDATING) {
		return true, nil
	}

	patch, err := createPatch(resp, &cr.Spec.ForProvider)
	if err != nil {
		return false, err
	}

	return cmp.Equal(&svcapitypes.TableParameters{}, patch,
		cmpopts.IgnoreTypes(&xpv1.Reference{}, &xpv1.Selector{}, []xpv1.Reference{}),
		cmpopts.IgnoreFields(svcapitypes.TableParameters{}, "Region", "Tags", "KeySchema", "LocalSecondaryIndexes", "CustomTableParameters")), nil
}

type updateClient struct {
	client svcsdkapi.DynamoDBAPI
}

func (e *updateClient) preUpdate(ctx context.Context, cr *svcapitypes.Table, u *svcsdk.UpdateTableInput) error {

	filtered := &svcsdk.UpdateTableInput{TableName: aws.String(meta.GetExternalName(cr))}

	// The AWS API requires us to do one kind of update at a time per
	// https://github.com/aws/aws-sdk-go/blob/v1.34.32/service/dynamodb/api.go#L5605
	// This means that we need to return a filtered UpdateTableInput that
	// contains at most one thing that needs updating. In order to be
	// eventually consistent (i.e. to eventually update all the things, one
	// on each reconcile pass) we need to determine the 'next' thing to
	// update on each pass. This means we need to diff actual vs desired
	// state here inside preUpdate. Unfortunately we read the actual state
	// during Observe, but don't typically pass it to update. We could stash
	// the observed state in a cache during postObserve then read it here,
	// but we typically prefer to be as stateless as possible even if it
	// means redundant API calls.
	out, err := e.client.DescribeTableWithContext(ctx, &svcsdk.DescribeTableInput{TableName: aws.String(meta.GetExternalName(cr))})
	if err != nil {
		return aws.Wrap(err, errDescribe)
	}

	p, err := createPatch(out, &cr.Spec.ForProvider)
	if err != nil {
		return err
	}

	// TODO(muvaf): Implement ReplicaUpdates and GlobalSecondaryIndexUpdates.
	switch {
	case p.ProvisionedThroughput != nil:
		filtered.ProvisionedThroughput = u.ProvisionedThroughput
	case p.StreamSpecification != nil:
		// NOTE(muvaf): Unless StreamEnabled is changed, updating stream
		// specification won't work.
		filtered.StreamSpecification = u.StreamSpecification
	case p.SSESpecification != nil:
		// NOTE(negz): Attempting to update the KMSMasterKeyId to its
		// current value returns an error
		filtered.SSESpecification = &svcsdk.SSESpecification{
			Enabled: u.SSESpecification.Enabled,
			SSEType: u.SSESpecification.SSEType,
		}
		if p.SSESpecification.KMSMasterKeyID != nil {
			filtered.SSESpecification.KMSMasterKeyId = u.SSESpecification.KMSMasterKeyId
		}
	case p.BillingMode != nil:
		filtered.BillingMode = u.BillingMode
	}

	gsiList := generateGlobalSecondaryIndexesUpdate(p)

	var gsiUpdateList []*svcsdk.GlobalSecondaryIndexUpdate

	for _, gsi := range gsiList {
		if searchGlobalSecondaryIndex(out.Table.GlobalSecondaryIndexes, gsi.IndexName) {
			gsiUpdateList = append(gsiUpdateList, &svcsdk.GlobalSecondaryIndexUpdate{Update: setUpdateGlobalSecondaryIndexAction(gsi)})
		} else {
			gsiUpdateList = append(gsiUpdateList, &svcsdk.GlobalSecondaryIndexUpdate{Create: setCreateGlobalSecondaryIndexAction(gsi)})
		}
	}

	filtered.SetGlobalSecondaryIndexUpdates(gsiUpdateList)

	*u = *filtered

	return nil
}

func generateGlobalSecondaryIndexesUpdate(p *svcapitypes.TableParameters) []*svcsdk.GlobalSecondaryIndex {
	gsiList := []*svcsdk.GlobalSecondaryIndex{}
	for _, gsiIter := range p.GlobalSecondaryIndexes {
		gsi := &svcsdk.GlobalSecondaryIndex{}
		if gsiIter.IndexName != nil {
			gsi.SetIndexName(*gsiIter.IndexName)
		}
		if gsiIter.KeySchema != nil {
			keySchemaList := []*svcsdk.KeySchemaElement{}
			for _, keySchemaIter := range gsiIter.KeySchema {
				keySchema := &svcsdk.KeySchemaElement{}
				if keySchemaIter.AttributeName != nil {
					keySchema.SetAttributeName(*keySchemaIter.AttributeName)
				}
				if keySchemaIter.KeyType != nil {
					keySchema.SetKeyType(*keySchemaIter.KeyType)
				}
				keySchemaList = append(keySchemaList, keySchema)
			}
			gsi.SetKeySchema(keySchemaList)
		}
		if gsiIter.Projection != nil {
			projection := &svcsdk.Projection{}
			if gsiIter.Projection.NonKeyAttributes != nil {
				nonKeyAttrList := []*string{}
				for _, nonKeyAttrIter := range gsiIter.Projection.NonKeyAttributes {
					var nonKeyAttr string
					nonKeyAttr = *nonKeyAttrIter
					nonKeyAttrList = append(nonKeyAttrList, &nonKeyAttr)
				}
				projection.SetNonKeyAttributes(nonKeyAttrList)
			}
			if gsiIter.Projection.ProjectionType != nil {
				projection.SetProjectionType(*gsiIter.Projection.ProjectionType)
			}
			gsi.SetProjection(projection)
		}
		if gsiIter.ProvisionedThroughput != nil {
			provisionedThroughput := &svcsdk.ProvisionedThroughput{}
			if gsiIter.ProvisionedThroughput.ReadCapacityUnits != nil {
				provisionedThroughput.SetReadCapacityUnits(*gsiIter.ProvisionedThroughput.ReadCapacityUnits)
			}
			if gsiIter.ProvisionedThroughput.WriteCapacityUnits != nil {
				provisionedThroughput.SetWriteCapacityUnits(*gsiIter.ProvisionedThroughput.WriteCapacityUnits)
			}
			gsi.SetProvisionedThroughput(provisionedThroughput)
		}
		gsiList = append(gsiList, gsi)
	}

	return gsiList
}

func setCreateGlobalSecondaryIndexAction(gsi *svcsdk.GlobalSecondaryIndex) *svcsdk.CreateGlobalSecondaryIndexAction {
	action := &svcsdk.CreateGlobalSecondaryIndexAction{}

	action.SetIndexName(*gsi.IndexName)
	action.SetKeySchema(gsi.KeySchema)
	action.SetProjection(gsi.Projection)
	action.SetProvisionedThroughput(gsi.ProvisionedThroughput)

	return action
}

func setUpdateGlobalSecondaryIndexAction(gsi *svcsdk.GlobalSecondaryIndex) (action *svcsdk.UpdateGlobalSecondaryIndexAction) {
	action.SetIndexName(*gsi.IndexName)
	action.SetProvisionedThroughput(gsi.ProvisionedThroughput)

	return
}

func setDeleteGlobalSecondaryIndexAction(gsi *svcsdk.GlobalSecondaryIndex) (action *svcsdk.DeleteGlobalSecondaryIndexAction) {
	action.SetIndexName(*gsi.IndexName)

	return
}

func searchGlobalSecondaryIndex(gsiList []*svcsdk.GlobalSecondaryIndexDescription, indexName *string) bool {
	for _, gsi := range gsiList {
		if gsi.IndexName == indexName {
			return true
		}
	}

	return false
}
