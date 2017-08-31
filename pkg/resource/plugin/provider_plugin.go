// Copyright 2016-2017, Pulumi Corporation.  All rights reserved.

package plugin

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-fabric/pkg/resource"
	"github.com/pulumi/pulumi-fabric/pkg/tokens"
	"github.com/pulumi/pulumi-fabric/pkg/util/contract"
	lumirpc "github.com/pulumi/pulumi-fabric/sdk/proto/go"
)

const ProviderPluginPrefix = "lumi-resource-"

// provider reflects a resource plugin, loaded dynamically for a single package.
type provider struct {
	ctx    *Context
	pkg    tokens.Package
	plug   *plugin
	client lumirpc.ResourceProviderClient
}

// NewProvider attempts to bind to a given package's resource plugin and then creates a gRPC connection to it.  If the
// plugin could not be found, or an error occurs while creating the child process, an error is returned.
func NewProvider(host Host, ctx *Context, pkg tokens.Package) (Provider, error) {
	// Go ahead and attempt to load the plugin from the PATH.
	srvexe := ProviderPluginPrefix + strings.Replace(string(pkg), tokens.QNameDelimiter, "_", -1)
	plug, err := newPlugin(host, ctx, []string{srvexe}, fmt.Sprintf("resource[%v]", pkg))
	if err != nil {
		return nil, err
	}

	return &provider{
		ctx:    ctx,
		pkg:    pkg,
		plug:   plug,
		client: lumirpc.NewResourceProviderClient(plug.Conn),
	}, nil
}

func (p *provider) Pkg() tokens.Package { return p.pkg }

// Check validates that the given property bag is valid for a resource of the given type.
func (p *provider) Check(urn resource.URN, props resource.PropertyMap) (resource.PropertyMap, []CheckFailure, error) {
	glog.V(7).Infof("resource[%v].Check(urn=%v,#props=%v) executing", p.pkg, urn, len(props))
	req := &lumirpc.CheckRequest{
		Urn:        string(urn),
		Properties: MarshalProperties(props, MarshalOptions{}),
	}

	resp, err := p.client.Check(p.ctx.Request(), req)
	if err != nil {
		glog.V(7).Infof("resource[%v].Check(urn=%v,...) failed: err=%v", p.pkg, urn, err)
		return nil, nil, err
	}

	// Unmarshal any defaults.
	var defaults resource.PropertyMap
	if defs := resp.GetDefaults(); defs != nil {
		defaults = UnmarshalProperties(defs, MarshalOptions{})
	}

	// And now any properties that failed verification.
	var failures []CheckFailure
	for _, failure := range resp.GetFailures() {
		failures = append(failures, CheckFailure{resource.PropertyKey(failure.Property), failure.Reason})
	}

	glog.V(7).Infof("resource[%v].Check(urn=%v,...) success: defs=#%v failures=#%v",
		p.pkg, urn, len(defaults), len(failures))
	return defaults, failures, nil
}

// Diff checks what impacts a hypothetical update will have on the resource's properties.
func (p *provider) Diff(urn resource.URN, id resource.ID,
	olds resource.PropertyMap, news resource.PropertyMap) (DiffResult, error) {
	contract.Assert(urn != "")
	contract.Assert(id != "")
	contract.Assert(news != nil)
	contract.Assert(olds != nil)
	glog.V(7).Infof("resource[%v].Diff(id=%v,urn=%v,#olds=%v,#news=%v) executing",
		p.pkg, id, urn, len(olds), len(news))

	resp, err := p.client.Diff(p.ctx.Request(), &lumirpc.DiffRequest{
		Id:   string(id),
		Urn:  string(urn),
		Olds: MarshalProperties(olds, MarshalOptions{DisallowUnknowns: true}),
		News: MarshalProperties(news, MarshalOptions{}),
	})
	if err != nil {
		glog.V(7).Infof("resource[%v].Diff(id=%v,urn=%v,...) failed: %v", p.pkg, id, urn, err)
		return DiffResult{}, err
	}

	var replaces []resource.PropertyKey
	for _, replace := range resp.GetReplaces() {
		replaces = append(replaces, resource.PropertyKey(replace))
	}
	glog.V(7).Infof("resource[%v].Update(id=%v,urn=%v,...) success: #replaces=%v", p.pkg, id, urn, len(replaces))
	return DiffResult{ReplaceKeys: replaces}, nil
}

// Create allocates a new instance of the provided resource and assigns its unique resource.ID and outputs afterwards.
func (p *provider) Create(urn resource.URN, props resource.PropertyMap) (resource.ID,
	resource.PropertyMap, resource.Status, error) {
	contract.Assert(urn != "")
	contract.Assert(props != nil)
	glog.V(7).Infof("resource[%v].Create(urn=%v,#props=%v) executing", p.pkg, urn, len(props))
	req := &lumirpc.CreateRequest{
		Urn:        string(urn),
		Properties: MarshalProperties(props, MarshalOptions{DisallowUnknowns: true}),
	}

	resp, err := p.client.Create(p.ctx.Request(), req)
	if err != nil {
		glog.V(7).Infof("resource[%v].Create(urn=%v,...) failed: err=%v", p.pkg, urn, err)
		return "", nil, resource.StatusUnknown, err
	}

	id := resource.ID(resp.GetId())
	if id == "" {
		return "", nil, resource.StatusUnknown,
			errors.Errorf("plugin for package '%v' returned empty resource.ID from create '%v'", p.pkg, urn)
	}
	outs := UnmarshalProperties(resp.GetProperties(), MarshalOptions{})

	glog.V(7).Infof("resource[%v].Create(urn=%v,...) success: id=%v; #outs=%v", p.pkg, urn, id, len(outs))
	return id, outs, resource.StatusOK, nil
}

// Update updates an existing resource with new values.
func (p *provider) Update(urn resource.URN, id resource.ID,
	olds resource.PropertyMap, news resource.PropertyMap) (resource.PropertyMap, resource.Status, error) {
	contract.Assert(urn != "")
	contract.Assert(id != "")
	contract.Assert(news != nil)
	contract.Assert(olds != nil)

	glog.V(7).Infof("resource[%v].Update(id=%v,urn=%v,#olds=%v,#news=%v) executing",
		p.pkg, id, urn, len(olds), len(news))
	req := &lumirpc.UpdateRequest{
		Id:   string(id),
		Urn:  string(urn),
		Olds: MarshalProperties(olds, MarshalOptions{DisallowUnknowns: true}),
		News: MarshalProperties(news, MarshalOptions{DisallowUnknowns: true}),
	}

	resp, err := p.client.Update(p.ctx.Request(), req)
	if err != nil {
		glog.V(7).Infof("resource[%v].Update(id=%v,urn=%v,...) failed: %v", p.pkg, id, urn, err)
		return nil, resource.StatusUnknown, err
	}
	outs := UnmarshalProperties(resp.GetProperties(), MarshalOptions{})

	glog.V(7).Infof("resource[%v].Update(id=%v,urn=%v,...) success; #out=%v", p.pkg, id, urn, len(outs))
	return outs, resource.StatusOK, nil
}

// Delete tears down an existing resource.
func (p *provider) Delete(urn resource.URN, id resource.ID, props resource.PropertyMap) (resource.Status, error) {
	contract.Assert(urn != "")
	contract.Assert(id != "")

	glog.V(7).Infof("resource[%v].Delete(id=%v,urn=%v) executing", p.pkg, id, urn)
	req := &lumirpc.DeleteRequest{
		Id:         string(id),
		Urn:        string(urn),
		Properties: MarshalProperties(props, MarshalOptions{DisallowUnknowns: true}),
	}

	if _, err := p.client.Delete(p.ctx.Request(), req); err != nil {
		glog.V(7).Infof("resource[%v].Delete(id=%v,urn=%v) failed: %v", p.pkg, id, urn, err)
		return resource.StatusUnknown, err
	}

	glog.V(7).Infof("resource[%v].Delete(id=%v,urn=%v) success", p.pkg, id, urn)
	return resource.StatusOK, nil
}

// Close tears down the underlying plugin RPC connection and process.
func (p *provider) Close() error {
	return p.plug.Close()
}
