/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tasks

import (
	"reflect"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/domainlayer"
	"github.com/apache/incubator-devlake/core/models/domainlayer/didgen"
	"github.com/apache/incubator-devlake/core/models/domainlayer/ticket"
	"github.com/apache/incubator-devlake/core/plugin"
	helper "github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	"github.com/apache/incubator-devlake/plugins/asana/models"
)

var _ plugin.SubTaskEntryPoint = ConvertTask

var ConvertTaskMeta = plugin.SubTaskMeta{
	Name:             "ConvertTask",
	EntryPoint:       ConvertTask,
	EnabledByDefault: true,
	Description:      "Convert tool layer Asana tasks into domain layer issues and board_issues",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
}

func ConvertTask(taskCtx plugin.SubTaskContext) errors.Error {
	rawDataSubTaskArgs, data := CreateRawDataSubTaskArgs(taskCtx, rawTaskTable)
	db := taskCtx.GetDal()
	connectionId := data.Options.ConnectionId
	projectId := data.Options.ProjectId

	// Get scope config for type/status mappings
	scopeConfig := getScopeConfig(taskCtx)

	clauses := []dal.Clause{
		dal.From(&models.AsanaTask{}),
		dal.Where("connection_id = ? AND project_gid = ?", connectionId, projectId),
	}
	cursor, err := db.Cursor(clauses...)
	if err != nil {
		return err
	}
	defer cursor.Close()

	taskIdGen := didgen.NewDomainIdGenerator(&models.AsanaTask{})
	boardIdGen := didgen.NewDomainIdGenerator(&models.AsanaProject{})
	accountIdGen := didgen.NewDomainIdGenerator(&models.AsanaUser{})

	converter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: *rawDataSubTaskArgs,
		InputRowType:       reflect.TypeOf(models.AsanaTask{}),
		Input:              cursor,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			toolTask := inputRow.(*models.AsanaTask)

			// Map type and status using scope config
			stdType, stdStatus := getStdTypeAndStatus(toolTask, scopeConfig)

			domainIssue := &ticket.Issue{
				DomainEntity:   domainlayer.DomainEntity{Id: taskIdGen.Generate(toolTask.ConnectionId, toolTask.Gid)},
				IssueKey:       toolTask.Gid,
				Title:          toolTask.Name,
				Description:    toolTask.Notes,
				Url:            toolTask.PermalinkUrl,
				Type:           stdType,
				OriginalType:   toolTask.ResourceSubtype,
				Status:         stdStatus,
				OriginalStatus: getOriginalStatus(toolTask),
				Priority:       toolTask.Priority,
				StoryPoint:     toolTask.StoryPoint,
				CreatedDate:    &toolTask.CreatedAt,
				UpdatedDate:    toolTask.ModifiedAt,
				ResolutionDate: toolTask.CompletedAt,
				DueDate:        toolTask.DueOn,
				CreatorName:    toolTask.CreatorName,
				AssigneeName:   toolTask.AssigneeName,
				LeadTimeMinutes: toolTask.LeadTimeMinutes,
			}

			// Set creator and assignee IDs
			if toolTask.CreatorGid != "" {
				domainIssue.CreatorId = accountIdGen.Generate(connectionId, toolTask.CreatorGid)
			}
			if toolTask.AssigneeGid != "" {
				domainIssue.AssigneeId = accountIdGen.Generate(connectionId, toolTask.AssigneeGid)
			}

			// Set parent issue ID if this is a subtask
			if toolTask.ParentGid != "" {
				domainIssue.ParentIssueId = taskIdGen.Generate(connectionId, toolTask.ParentGid)
				// If no type mapping and has parent, it's a subtask
				if stdType == "" {
					domainIssue.Type = ticket.SUBTASK
				}
			}

			// Set subtask flag
			domainIssue.IsSubtask = toolTask.ParentGid != ""

			var result []interface{}
			result = append(result, domainIssue)

			// Create board issue relationship
			boardId := boardIdGen.Generate(connectionId, toolTask.ProjectGid)
			boardIssue := &ticket.BoardIssue{
				BoardId: boardId,
				IssueId: domainIssue.Id,
			}
			result = append(result, boardIssue)

			// Create issue assignee if assignee exists
			if toolTask.AssigneeGid != "" {
				issueAssignee := &ticket.IssueAssignee{
					IssueId:      domainIssue.Id,
					AssigneeId:   domainIssue.AssigneeId,
					AssigneeName: toolTask.AssigneeName,
				}
				result = append(result, issueAssignee)
			}

			return result, nil
		},
	})
	if err != nil {
		return err
	}
	return converter.Execute()
}

// getScopeConfig retrieves the scope config for transformation rules
func getScopeConfig(taskCtx plugin.SubTaskContext) *models.AsanaScopeConfig {
	if taskCtx.GetData() == nil {
		return nil
	}
	data := taskCtx.GetData().(*AsanaTaskData)
	if data.Options.ScopeConfigId == 0 {
		return nil
	}
	db := taskCtx.GetDal()
	var scopeConfig models.AsanaScopeConfig
	err := db.First(&scopeConfig, dal.Where("id = ?", data.Options.ScopeConfigId))
	if err != nil {
		return nil
	}
	return &scopeConfig
}

// getStdTypeAndStatus maps Asana task to standard type and status
func getStdTypeAndStatus(task *models.AsanaTask, scopeConfig *models.AsanaScopeConfig) (string, string) {
	stdType := ""
	stdStatus := ""

	// Default status based on completion
	if task.Completed {
		stdStatus = ticket.DONE
	} else {
		stdStatus = ticket.TODO
	}

	// Use pre-computed values if available
	if task.StdType != "" {
		stdType = task.StdType
	}
	if task.StdStatus != "" {
		stdStatus = task.StdStatus
	}

	// Apply scope config mappings if available
	if scopeConfig != nil && scopeConfig.TypeMappings != nil {
		// Map resource_subtype to standard type
		if typeMapping, ok := scopeConfig.TypeMappings[task.ResourceSubtype]; ok {
			if typeMapping.StandardType != "" {
				stdType = typeMapping.StandardType
			}
			// Map section name to status if available
			if task.SectionName != "" && typeMapping.StatusMappings != nil {
				if statusMapping, ok := typeMapping.StatusMappings[task.SectionName]; ok {
					if statusMapping.StandardStatus != "" {
						stdStatus = statusMapping.StandardStatus
					}
				}
			}
		}
	}

	// Default type mapping based on resource_subtype
	// Asana terminology: default_task, milestone, approval, section
	if stdType == "" {
		switch task.ResourceSubtype {
		case "milestone":
			stdType = ticket.REQUIREMENT // Milestones represent key deliverables
		case "approval":
			stdType = ticket.TASK
		case "section":
			stdType = ticket.TASK // Sections are just groupings, not tasks themselves
		default:
			if task.ParentGid != "" {
				stdType = ticket.SUBTASK
			} else {
				stdType = ticket.TASK
			}
		}
	}

	return stdType, stdStatus
}

// getOriginalStatus returns the original status string
func getOriginalStatus(task *models.AsanaTask) string {
	if task.Completed {
		return "completed"
	}
	if task.SectionName != "" {
		return task.SectionName
	}
	return "incomplete"
}

