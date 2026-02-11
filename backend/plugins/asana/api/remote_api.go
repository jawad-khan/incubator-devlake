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

package api

import (
	"fmt"
	"net/url"

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	dsmodels "github.com/apache/incubator-devlake/helpers/pluginhelper/api/models"
	"github.com/apache/incubator-devlake/plugins/asana/models"
)

type AsanaRemotePagination struct {
	Offset string `json:"offset"`
	Limit  int    `json:"limit"`
}

type asanaWorkspaceResponse struct {
	Gid          string `json:"gid"`
	Name         string `json:"name"`
	ResourceType string `json:"resource_type"`
}

type asanaWorkspacesListResponse struct {
	Data []asanaWorkspaceResponse `json:"data"`
	NextPage *struct {
		Offset string `json:"offset"`
		Path   string `json:"path"`
		URI    string `json:"uri"`
	} `json:"next_page"`
}

type asanaProjectResponse struct {
	Gid          string `json:"gid"`
	Name         string `json:"name"`
	ResourceType string `json:"resource_type"`
	Archived     bool   `json:"archived"`
	PermalinkUrl string `json:"permalink_url"`
	Workspace    *struct {
		Gid string `json:"gid"`
	} `json:"workspace"`
}

type asanaProjectsListResponse struct {
	Data     []asanaProjectResponse `json:"data"`
	NextPage *struct {
		Offset string `json:"offset"`
		Path   string `json:"path"`
		URI    string `json:"uri"`
	} `json:"next_page"`
}

func listAsanaRemoteScopes(
	connection *models.AsanaConnection,
	apiClient plugin.ApiClient,
	groupId string,
	page AsanaRemotePagination,
) (
	children []dsmodels.DsRemoteApiScopeListEntry[models.AsanaProject],
	nextPage *AsanaRemotePagination,
	err errors.Error,
) {
	if page.Limit == 0 {
		page.Limit = 100
	}

	// If no groupId, list workspaces as groups first
	if groupId == "" {
		return listAsanaWorkspaces(apiClient, page)
	}

	// If groupId is provided, it's a workspace GID — list projects in that workspace
	return listAsanaProjects(apiClient, groupId, page)
}

func listAsanaWorkspaces(
	apiClient plugin.ApiClient,
	page AsanaRemotePagination,
) (
	children []dsmodels.DsRemoteApiScopeListEntry[models.AsanaProject],
	nextPage *AsanaRemotePagination,
	err errors.Error,
) {
	query := url.Values{}
	query.Set("limit", fmt.Sprintf("%d", page.Limit))
	query.Set("opt_fields", "name,resource_type")
	if page.Offset != "" {
		query.Set("offset", page.Offset)
	}

	res, err := apiClient.Get("workspaces", query, nil)
	if err != nil {
		return nil, nil, errors.Default.Wrap(err, "failed to fetch workspaces from Asana API")
	}

	var response asanaWorkspacesListResponse
	err = api.UnmarshalResponse(res, &response)
	if err != nil {
		return nil, nil, errors.Default.Wrap(err, "failed to unmarshal Asana workspaces response")
	}

	for _, workspace := range response.Data {
		children = append(children, dsmodels.DsRemoteApiScopeListEntry[models.AsanaProject]{
			Type:     api.RAS_ENTRY_TYPE_GROUP,
			Id:       workspace.Gid,
			Name:     workspace.Name,
			FullName: workspace.Name,
		})
	}

	if response.NextPage != nil && response.NextPage.Offset != "" {
		nextPage = &AsanaRemotePagination{
			Offset: response.NextPage.Offset,
			Limit:  page.Limit,
		}
	}

	return children, nextPage, nil
}

func listAsanaProjects(
	apiClient plugin.ApiClient,
	workspaceGid string,
	page AsanaRemotePagination,
) (
	children []dsmodels.DsRemoteApiScopeListEntry[models.AsanaProject],
	nextPage *AsanaRemotePagination,
	err errors.Error,
) {
	query := url.Values{}
	query.Set("limit", fmt.Sprintf("%d", page.Limit))
	query.Set("opt_fields", "name,resource_type,archived,permalink_url,workspace")
	if page.Offset != "" {
		query.Set("offset", page.Offset)
	}

	apiPath := fmt.Sprintf("workspaces/%s/projects", workspaceGid)
	res, err := apiClient.Get(apiPath, query, nil)
	if err != nil {
		return nil, nil, errors.Default.Wrap(err, "failed to fetch projects from Asana API")
	}

	var response asanaProjectsListResponse
	err = api.UnmarshalResponse(res, &response)
	if err != nil {
		return nil, nil, errors.Default.Wrap(err, "failed to unmarshal Asana projects response")
	}

	for _, project := range response.Data {
		workspaceGidVal := workspaceGid
		if project.Workspace != nil {
			workspaceGidVal = project.Workspace.Gid
		}
		children = append(children, dsmodels.DsRemoteApiScopeListEntry[models.AsanaProject]{
			Type:     api.RAS_ENTRY_TYPE_SCOPE,
			Id:       project.Gid,
			Name:     project.Name,
			FullName: project.Name,
			Data: &models.AsanaProject{
				Gid:          project.Gid,
				Name:         project.Name,
				ResourceType: project.ResourceType,
				Archived:     project.Archived,
				PermalinkUrl: project.PermalinkUrl,
				WorkspaceGid: workspaceGidVal,
			},
		})
	}

	if response.NextPage != nil && response.NextPage.Offset != "" {
		nextPage = &AsanaRemotePagination{
			Offset: response.NextPage.Offset,
			Limit:  page.Limit,
		}
	}

	return children, nextPage, nil
}

// RemoteScopes list all available scopes (projects) for this connection
// @Summary list all available scopes (projects) for this connection
// @Description list all available scopes (projects) for this connection
// @Tags plugins/asana
// @Accept application/json
// @Param connectionId path int false "connection ID"
// @Param groupId query string false "group ID"
// @Param pageToken query string false "page Token"
// @Success 200  {object} RemoteScopesOutput
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/asana/connections/{connectionId}/remote-scopes [GET]
func RemoteScopes(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return raScopeList.Get(input)
}

func Proxy(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return raProxy.Proxy(input)
}
