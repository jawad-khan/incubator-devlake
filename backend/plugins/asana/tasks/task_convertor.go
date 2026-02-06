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

	converter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: *rawDataSubTaskArgs,
		InputRowType:       reflect.TypeOf(models.AsanaTask{}),
		Input:              cursor,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			toolTask := inputRow.(*models.AsanaTask)
			domainIssue := &ticket.Issue{
				DomainEntity:   domainlayer.DomainEntity{Id: taskIdGen.Generate(toolTask.ConnectionId, toolTask.Gid)},
				IssueKey:       toolTask.Gid,
				Title:          toolTask.Name,
				Description:    toolTask.Notes,
				Url:            toolTask.PermalinkUrl,
				CreatedDate:    &toolTask.CreatedAt,
				UpdatedDate:    toolTask.ModifiedAt,
				ResolutionDate: toolTask.CompletedAt,
				DueDate:        toolTask.DueOn,
				CreatorName:    toolTask.CreatorName,
				AssigneeName:   toolTask.AssigneeName,
				OriginalType:   toolTask.ResourceSubtype,
			}
			if toolTask.Completed {
				domainIssue.Status = ticket.DONE
				domainIssue.OriginalStatus = "completed"
			} else {
				domainIssue.Status = ticket.TODO
				domainIssue.OriginalStatus = "incomplete"
			}
			boardId := boardIdGen.Generate(connectionId, toolTask.ProjectGid)
			boardIssue := &ticket.BoardIssue{
				BoardId: boardId,
				IssueId: domainIssue.Id,
			}
			return []interface{}{domainIssue, boardIssue}, nil
		},
	})
	if err != nil {
		return err
	}
	return converter.Execute()
}
