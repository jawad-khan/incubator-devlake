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

package models

import (
	"github.com/apache/incubator-devlake/core/models/common"
)

// StatusMapping maps Asana statuses to standard statuses
type AsanaStatusMapping struct {
	StandardStatus string `json:"standardStatus"`
}

// AsanaStatusMappings is a map of section/status names to their mappings
type AsanaStatusMappings map[string]AsanaStatusMapping

// AsanaTypeMapping maps Asana task types to standard types with their status mappings
type AsanaTypeMapping struct {
	StandardType   string              `json:"standardType"`
	StatusMappings AsanaStatusMappings `json:"statusMappings"`
}

type AsanaScopeConfig struct {
	common.ScopeConfig `mapstructure:",squash" json:",inline" gorm:"embedded"`

	// Type and Status Mappings (like Jira)
	// Maps Asana resource_subtype (default_task, milestone, section, approval) to standard types
	// Standard types: REQUIREMENT, BUG, INCIDENT, EPIC, TASK, SUBTASK
	TypeMappings map[string]AsanaTypeMapping `mapstructure:"typeMappings,omitempty" json:"typeMappings" gorm:"type:json;serializer:json"`

	// Application type for categorization
	ApplicationType string `mapstructure:"applicationType,omitempty" json:"applicationType" gorm:"type:varchar(255)"`

	// Story Point field - custom field name/gid that contains story points
	StoryPointField string `mapstructure:"storyPointField,omitempty" json:"storyPointField" gorm:"type:varchar(255)"`

	// Priority field - custom field name/gid that contains priority
	PriorityField string `mapstructure:"priorityField,omitempty" json:"priorityField" gorm:"type:varchar(255)"`

	// Epic field - custom field name/gid that links tasks to epics
	EpicField string `mapstructure:"epicField,omitempty" json:"epicField" gorm:"type:varchar(255)"`

	// Severity field - custom field name/gid for severity (used for bugs/incidents)
	SeverityField string `mapstructure:"severityField,omitempty" json:"severityField" gorm:"type:varchar(255)"`

	// Due date handling
	DueDateField string `mapstructure:"dueDateField,omitempty" json:"dueDateField" gorm:"type:varchar(255)"`
}

func (AsanaScopeConfig) TableName() string {
	return "_tool_asana_scope_configs"
}

func (a *AsanaScopeConfig) SetConnectionId(c *AsanaScopeConfig, connectionId uint64) {
	c.ConnectionId = connectionId
	c.ScopeConfig.ConnectionId = connectionId
}
