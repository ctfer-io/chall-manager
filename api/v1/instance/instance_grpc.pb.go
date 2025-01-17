// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             (unknown)
// source: api/v1/instance/instance.proto

package instance

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	InstanceManager_CreateInstance_FullMethodName   = "/api.v1.instance.InstanceManager/CreateInstance"
	InstanceManager_RetrieveInstance_FullMethodName = "/api.v1.instance.InstanceManager/RetrieveInstance"
	InstanceManager_QueryInstance_FullMethodName    = "/api.v1.instance.InstanceManager/QueryInstance"
	InstanceManager_RenewInstance_FullMethodName    = "/api.v1.instance.InstanceManager/RenewInstance"
	InstanceManager_DeleteInstance_FullMethodName   = "/api.v1.instance.InstanceManager/DeleteInstance"
)

// InstanceManagerClient is the client API for InstanceManager service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// The InstanceManager handles the charge of spinning up challenge
// instances (require it to be stored in the ChallengeStore).
// Through this manager, the instance implements all CRUD operations necessary
// to handle a lifecycle.
type InstanceManagerClient interface {
	// Spins up a challenge instance, iif the challenge is registered
	// and no instance is yet running.
	CreateInstance(ctx context.Context, in *CreateInstanceRequest, opts ...grpc.CallOption) (*Instance, error)
	// Once created, you can retrieve the instance information.
	// If it has not been created yet, returns an error.
	RetrieveInstance(ctx context.Context, in *RetrieveInstanceRequest, opts ...grpc.CallOption) (*Instance, error)
	// Query all instances that matches the request parameters.
	// Especially usefull to query all the instances of a source_id.
	QueryInstance(ctx context.Context, in *QueryInstanceRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[Instance], error)
	// Once an instance is spinned up, it will have a lifetime.
	// Passed it, it will exprie i.e. will be deleted as soon as possible
	// by the chall-manager-janitor.
	// To increase this lifetime, a player can ask to renew it. This will
	// set the until date to the request time more the challenge timeout.
	RenewInstance(ctx context.Context, in *RenewInstanceRequest, opts ...grpc.CallOption) (*Instance, error)
	// After completion, the challenge instance is no longer required.
	// This spins down the instance and removes if from filesystem.
	DeleteInstance(ctx context.Context, in *DeleteInstanceRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type instanceManagerClient struct {
	cc grpc.ClientConnInterface
}

func NewInstanceManagerClient(cc grpc.ClientConnInterface) InstanceManagerClient {
	return &instanceManagerClient{cc}
}

func (c *instanceManagerClient) CreateInstance(ctx context.Context, in *CreateInstanceRequest, opts ...grpc.CallOption) (*Instance, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(Instance)
	err := c.cc.Invoke(ctx, InstanceManager_CreateInstance_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *instanceManagerClient) RetrieveInstance(ctx context.Context, in *RetrieveInstanceRequest, opts ...grpc.CallOption) (*Instance, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(Instance)
	err := c.cc.Invoke(ctx, InstanceManager_RetrieveInstance_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *instanceManagerClient) QueryInstance(ctx context.Context, in *QueryInstanceRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[Instance], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &InstanceManager_ServiceDesc.Streams[0], InstanceManager_QueryInstance_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[QueryInstanceRequest, Instance]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type InstanceManager_QueryInstanceClient = grpc.ServerStreamingClient[Instance]

func (c *instanceManagerClient) RenewInstance(ctx context.Context, in *RenewInstanceRequest, opts ...grpc.CallOption) (*Instance, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(Instance)
	err := c.cc.Invoke(ctx, InstanceManager_RenewInstance_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *instanceManagerClient) DeleteInstance(ctx context.Context, in *DeleteInstanceRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, InstanceManager_DeleteInstance_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// InstanceManagerServer is the server API for InstanceManager service.
// All implementations must embed UnimplementedInstanceManagerServer
// for forward compatibility.
//
// The InstanceManager handles the charge of spinning up challenge
// instances (require it to be stored in the ChallengeStore).
// Through this manager, the instance implements all CRUD operations necessary
// to handle a lifecycle.
type InstanceManagerServer interface {
	// Spins up a challenge instance, iif the challenge is registered
	// and no instance is yet running.
	CreateInstance(context.Context, *CreateInstanceRequest) (*Instance, error)
	// Once created, you can retrieve the instance information.
	// If it has not been created yet, returns an error.
	RetrieveInstance(context.Context, *RetrieveInstanceRequest) (*Instance, error)
	// Query all instances that matches the request parameters.
	// Especially usefull to query all the instances of a source_id.
	QueryInstance(*QueryInstanceRequest, grpc.ServerStreamingServer[Instance]) error
	// Once an instance is spinned up, it will have a lifetime.
	// Passed it, it will exprie i.e. will be deleted as soon as possible
	// by the chall-manager-janitor.
	// To increase this lifetime, a player can ask to renew it. This will
	// set the until date to the request time more the challenge timeout.
	RenewInstance(context.Context, *RenewInstanceRequest) (*Instance, error)
	// After completion, the challenge instance is no longer required.
	// This spins down the instance and removes if from filesystem.
	DeleteInstance(context.Context, *DeleteInstanceRequest) (*emptypb.Empty, error)
	mustEmbedUnimplementedInstanceManagerServer()
}

// UnimplementedInstanceManagerServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedInstanceManagerServer struct{}

func (UnimplementedInstanceManagerServer) CreateInstance(context.Context, *CreateInstanceRequest) (*Instance, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateInstance not implemented")
}
func (UnimplementedInstanceManagerServer) RetrieveInstance(context.Context, *RetrieveInstanceRequest) (*Instance, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RetrieveInstance not implemented")
}
func (UnimplementedInstanceManagerServer) QueryInstance(*QueryInstanceRequest, grpc.ServerStreamingServer[Instance]) error {
	return status.Errorf(codes.Unimplemented, "method QueryInstance not implemented")
}
func (UnimplementedInstanceManagerServer) RenewInstance(context.Context, *RenewInstanceRequest) (*Instance, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RenewInstance not implemented")
}
func (UnimplementedInstanceManagerServer) DeleteInstance(context.Context, *DeleteInstanceRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteInstance not implemented")
}
func (UnimplementedInstanceManagerServer) mustEmbedUnimplementedInstanceManagerServer() {}
func (UnimplementedInstanceManagerServer) testEmbeddedByValue()                         {}

// UnsafeInstanceManagerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to InstanceManagerServer will
// result in compilation errors.
type UnsafeInstanceManagerServer interface {
	mustEmbedUnimplementedInstanceManagerServer()
}

func RegisterInstanceManagerServer(s grpc.ServiceRegistrar, srv InstanceManagerServer) {
	// If the following call pancis, it indicates UnimplementedInstanceManagerServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&InstanceManager_ServiceDesc, srv)
}

func _InstanceManager_CreateInstance_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateInstanceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InstanceManagerServer).CreateInstance(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: InstanceManager_CreateInstance_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InstanceManagerServer).CreateInstance(ctx, req.(*CreateInstanceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _InstanceManager_RetrieveInstance_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RetrieveInstanceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InstanceManagerServer).RetrieveInstance(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: InstanceManager_RetrieveInstance_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InstanceManagerServer).RetrieveInstance(ctx, req.(*RetrieveInstanceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _InstanceManager_QueryInstance_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(QueryInstanceRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(InstanceManagerServer).QueryInstance(m, &grpc.GenericServerStream[QueryInstanceRequest, Instance]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type InstanceManager_QueryInstanceServer = grpc.ServerStreamingServer[Instance]

func _InstanceManager_RenewInstance_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RenewInstanceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InstanceManagerServer).RenewInstance(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: InstanceManager_RenewInstance_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InstanceManagerServer).RenewInstance(ctx, req.(*RenewInstanceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _InstanceManager_DeleteInstance_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteInstanceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(InstanceManagerServer).DeleteInstance(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: InstanceManager_DeleteInstance_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(InstanceManagerServer).DeleteInstance(ctx, req.(*DeleteInstanceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// InstanceManager_ServiceDesc is the grpc.ServiceDesc for InstanceManager service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var InstanceManager_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "api.v1.instance.InstanceManager",
	HandlerType: (*InstanceManagerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CreateInstance",
			Handler:    _InstanceManager_CreateInstance_Handler,
		},
		{
			MethodName: "RetrieveInstance",
			Handler:    _InstanceManager_RetrieveInstance_Handler,
		},
		{
			MethodName: "RenewInstance",
			Handler:    _InstanceManager_RenewInstance_Handler,
		},
		{
			MethodName: "DeleteInstance",
			Handler:    _InstanceManager_DeleteInstance_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "QueryInstance",
			Handler:       _InstanceManager_QueryInstance_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "api/v1/instance/instance.proto",
}
