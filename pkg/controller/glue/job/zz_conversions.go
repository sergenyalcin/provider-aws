/*
Copyright 2021 The Crossplane Authors.

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

// Code generated by ack-generate. DO NOT EDIT.

package job

import (
	"github.com/aws/aws-sdk-go/aws/awserr"
	svcsdk "github.com/aws/aws-sdk-go/service/glue"

	svcapitypes "github.com/crossplane/provider-aws/apis/glue/v1alpha1"
)

// NOTE(muvaf): We return pointers in case the function needs to start with an
// empty object, hence need to return a new pointer.

// GenerateGetJobInput returns input for read
// operation.
func GenerateGetJobInput(cr *svcapitypes.Job) *svcsdk.GetJobInput {
	res := &svcsdk.GetJobInput{}

	return res
}

// GenerateJob returns the current state in the form of *svcapitypes.Job.
func GenerateJob(resp *svcsdk.GetJobOutput) *svcapitypes.Job {
	cr := &svcapitypes.Job{}

	if resp.Job.Name != nil {
		cr.Status.AtProvider.Name = resp.Job.Name
	} else {
		cr.Status.AtProvider.Name = nil
	}

	return cr
}

// GenerateCreateJobInput returns a create input.
func GenerateCreateJobInput(cr *svcapitypes.Job) *svcsdk.CreateJobInput {
	res := &svcsdk.CreateJobInput{}

	if cr.Spec.ForProvider.AllocatedCapacity != nil {
		res.SetAllocatedCapacity(*cr.Spec.ForProvider.AllocatedCapacity)
	}
	if cr.Spec.ForProvider.Command != nil {
		f1 := &svcsdk.JobCommand{}
		if cr.Spec.ForProvider.Command.Name != nil {
			f1.SetName(*cr.Spec.ForProvider.Command.Name)
		}
		if cr.Spec.ForProvider.Command.PythonVersion != nil {
			f1.SetPythonVersion(*cr.Spec.ForProvider.Command.PythonVersion)
		}
		if cr.Spec.ForProvider.Command.ScriptLocation != nil {
			f1.SetScriptLocation(*cr.Spec.ForProvider.Command.ScriptLocation)
		}
		res.SetCommand(f1)
	}
	if cr.Spec.ForProvider.Connections != nil {
		f2 := &svcsdk.ConnectionsList{}
		if cr.Spec.ForProvider.Connections.Connections != nil {
			f2f0 := []*string{}
			for _, f2f0iter := range cr.Spec.ForProvider.Connections.Connections {
				var f2f0elem string
				f2f0elem = *f2f0iter
				f2f0 = append(f2f0, &f2f0elem)
			}
			f2.SetConnections(f2f0)
		}
		res.SetConnections(f2)
	}
	if cr.Spec.ForProvider.DefaultArguments != nil {
		f3 := map[string]*string{}
		for f3key, f3valiter := range cr.Spec.ForProvider.DefaultArguments {
			var f3val string
			f3val = *f3valiter
			f3[f3key] = &f3val
		}
		res.SetDefaultArguments(f3)
	}
	if cr.Spec.ForProvider.Description != nil {
		res.SetDescription(*cr.Spec.ForProvider.Description)
	}
	if cr.Spec.ForProvider.ExecutionProperty != nil {
		f5 := &svcsdk.ExecutionProperty{}
		if cr.Spec.ForProvider.ExecutionProperty.MaxConcurrentRuns != nil {
			f5.SetMaxConcurrentRuns(*cr.Spec.ForProvider.ExecutionProperty.MaxConcurrentRuns)
		}
		res.SetExecutionProperty(f5)
	}
	if cr.Spec.ForProvider.GlueVersion != nil {
		res.SetGlueVersion(*cr.Spec.ForProvider.GlueVersion)
	}
	if cr.Spec.ForProvider.LogURI != nil {
		res.SetLogUri(*cr.Spec.ForProvider.LogURI)
	}
	if cr.Spec.ForProvider.MaxCapacity != nil {
		res.SetMaxCapacity(*cr.Spec.ForProvider.MaxCapacity)
	}
	if cr.Spec.ForProvider.MaxRetries != nil {
		res.SetMaxRetries(*cr.Spec.ForProvider.MaxRetries)
	}
	if cr.Spec.ForProvider.NonOverridableArguments != nil {
		f10 := map[string]*string{}
		for f10key, f10valiter := range cr.Spec.ForProvider.NonOverridableArguments {
			var f10val string
			f10val = *f10valiter
			f10[f10key] = &f10val
		}
		res.SetNonOverridableArguments(f10)
	}
	if cr.Spec.ForProvider.NotificationProperty != nil {
		f11 := &svcsdk.NotificationProperty{}
		if cr.Spec.ForProvider.NotificationProperty.NotifyDelayAfter != nil {
			f11.SetNotifyDelayAfter(*cr.Spec.ForProvider.NotificationProperty.NotifyDelayAfter)
		}
		res.SetNotificationProperty(f11)
	}
	if cr.Spec.ForProvider.NumberOfWorkers != nil {
		res.SetNumberOfWorkers(*cr.Spec.ForProvider.NumberOfWorkers)
	}
	if cr.Spec.ForProvider.SecurityConfiguration != nil {
		res.SetSecurityConfiguration(*cr.Spec.ForProvider.SecurityConfiguration)
	}
	if cr.Spec.ForProvider.Tags != nil {
		f14 := map[string]*string{}
		for f14key, f14valiter := range cr.Spec.ForProvider.Tags {
			var f14val string
			f14val = *f14valiter
			f14[f14key] = &f14val
		}
		res.SetTags(f14)
	}
	if cr.Spec.ForProvider.Timeout != nil {
		res.SetTimeout(*cr.Spec.ForProvider.Timeout)
	}
	if cr.Spec.ForProvider.WorkerType != nil {
		res.SetWorkerType(*cr.Spec.ForProvider.WorkerType)
	}

	return res
}

// GenerateUpdateJobInput returns an update input.
func GenerateUpdateJobInput(cr *svcapitypes.Job) *svcsdk.UpdateJobInput {
	res := &svcsdk.UpdateJobInput{}

	return res
}

// GenerateDeleteJobInput returns a deletion input.
func GenerateDeleteJobInput(cr *svcapitypes.Job) *svcsdk.DeleteJobInput {
	res := &svcsdk.DeleteJobInput{}

	return res
}

// IsNotFound returns whether the given error is of type NotFound or not.
func IsNotFound(err error) bool {
	awsErr, ok := err.(awserr.Error)
	return ok && awsErr.Code() == "EntityNotFoundException"
}
