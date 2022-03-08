// Copyright (c) 2022 Cisco Systems, Inc. and its affiliates
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//     * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//
// The contents of this file are licensed under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance with the
// License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations under
// the License.

package arangodb

import (
	"strconv"

	"github.com/sbezverk/gobmp/pkg/message"
)

type lsPrefixArangoMessage struct {
	*message.LSPrefix
}

func (p *lsPrefixArangoMessage) MakeKey() string {
	mtid := 0
	if p.MTID != nil {
		mtid = int(p.MTID.MTID)
	}
	// The ls_prefix Key uses ProtocolID, DomainID, and Multi-Topology ID
	// to create unique Keys for DB entries in multi-area / multi-topology scenarios
	return strconv.Itoa(int(p.ProtocolID)) + "_" +
		strconv.Itoa(int(p.DomainID)) + "_" +
		strconv.Itoa(int(mtid)) + "_" +
		p.AreaID + "_" +
		strconv.Itoa(int(p.OSPFRouteType)) + "_" +
		p.Prefix + "_" + strconv.Itoa(int(p.PrefixLen)) + "_" +
		p.IGPRouterID
}
