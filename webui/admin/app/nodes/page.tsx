"use client";

import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { getStatusBadgeVariant } from "@/lib/status-badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Server,
  Plus,
  Search,
  Eye,
  ArrowDownToLine,
  RefreshCcw,
  Loader2,
  Pencil,
} from "lucide-react";
import {
  adminNodesApi,
  type Node,
  type NodeDetail,
  type CreateNodeRequest,
  type UpdateNodeRequest,
} from "@/lib/api-client";
import { useToast } from "@/components/ui/use-toast";
import { NodeCreateDialog, type CreateNodeFormData } from "@/components/nodes/NodeCreateDialog";
import { NodeEditDialog, type EditNodeFormData } from "@/components/nodes/NodeEditDialog";
import { NodeDetailDialog } from "@/components/nodes/NodeDetailDialog";

type DialogAction = "drain" | "failover" | null;

export default function NodesPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [nodes, setNodes] = useState<Node[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingId, setLoadingId] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogAction, setDialogAction] = useState<DialogAction>(null);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [detailDialogOpen, setDetailDialogOpen] = useState(false);
  const [editingNode, setEditingNode] = useState<NodeDetail | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const { toast } = useToast();

  const fetchNodes = useCallback(async () => {
    try {
      const response = await adminNodesApi.getNodes();
      setNodes(response.data || []);
    } catch (err) {
      toast({
        title: "Error",
        description: "Failed to load nodes.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    fetchNodes();
  }, [fetchNodes]);

  const filteredNodes = nodes.filter(
    (node) =>
      node.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      node.hostname.toLowerCase().includes(searchTerm.toLowerCase()) ||
      node.location.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const handleView = async (node: Node) => {
    setLoadingId(node.id);
    try {
      const nodeDetails = await adminNodesApi.getNode(node.id);
      setEditingNode(nodeDetails);
      setDetailDialogOpen(true);
    } catch (error) {
      toast({
        title: "Error",
        description: error instanceof Error ? error.message : "Failed to fetch node details",
        variant: "destructive",
      });
    } finally {
      setLoadingId(null);
    }
  };

  const handleEdit = async (node: Node) => {
    setLoadingId(node.id);
    try {
      const nodeDetails = await adminNodesApi.getNode(node.id);
      setEditingNode(nodeDetails);
      setEditDialogOpen(true);
    } catch (error) {
      toast({
        title: "Error",
        description: error instanceof Error ? error.message : "Failed to fetch node details",
        variant: "destructive",
      });
    } finally {
      setLoadingId(null);
    }
  };

  const handleCreate = async (data: CreateNodeFormData) => {
    setIsCreating(true);
    try {
      const request: CreateNodeRequest = {
        hostname: data.hostname,
        grpc_address: data.grpc_address,
        management_ip: data.management_ip,
        total_vcpu: data.total_vcpu,
        total_memory_mb: data.total_memory_mb,
        ipmi_address: data.ipmi_address || undefined,
        ipmi_username: data.ipmi_username || undefined,
        ipmi_password: data.ipmi_password || undefined,
      };
      await adminNodesApi.createNode(request);
      toast({
        title: "Node Registered",
        description: `Node ${data.hostname} has been registered successfully.`,
      });
      await fetchNodes();
    } finally {
      setIsCreating(false);
    }
  };

  const handleSave = async (data: EditNodeFormData) => {
    if (!editingNode) return;
    setIsSaving(true);
    try {
      const request: UpdateNodeRequest = {};
      if (data.grpc_address) request.grpc_address = data.grpc_address;
      if (data.total_vcpu) request.total_vcpu = data.total_vcpu;
      if (data.total_memory_mb) request.total_memory_mb = data.total_memory_mb;
      if (data.ipmi_address) request.ipmi_address = data.ipmi_address;

      await adminNodesApi.updateNode(editingNode.id, request);
      toast({
        title: "Node Updated",
        description: `Node ${editingNode.hostname} has been updated successfully.`,
      });
      await fetchNodes();
    } finally {
      setIsSaving(false);
    }
  };

  const openConfirmDialog = (node: Node, action: DialogAction) => {
    setSelectedNode(node);
    setDialogAction(action);
    setDialogOpen(true);
  };

  const handleConfirmAction = async () => {
    if (!selectedNode || !dialogAction) return;

    setLoadingId(selectedNode.id);
    setDialogOpen(false);

    try {
      if (dialogAction === "drain") {
        await adminNodesApi.drainNode(selectedNode.id);
        toast({
          title: "Node Drain Initiated",
          description: `VMs on ${selectedNode.name} are being migrated.`,
        });
      } else if (dialogAction === "failover") {
        await adminNodesApi.failoverNode(selectedNode.id);
        toast({
          title: "Failover Initiated",
          description: `Failover process started for ${selectedNode.name}.`,
        });
      }

      await fetchNodes();
    } catch (error) {
      toast({
        title: "Action Failed",
        description: error instanceof Error ? error.message : `Failed to ${dialogAction} node`,
        variant: "destructive",
      });
    } finally {
      setLoadingId(null);
      setSelectedNode(null);
      setDialogAction(null);
    }
  };

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Nodes</h1>
            <p className="text-muted-foreground">
              Manage hypervisor nodes and cluster capacity
            </p>
          </div>
          <Button onClick={() => setCreateDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Add Node
          </Button>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Hypervisor Cluster</CardTitle>
            <CardDescription>
              View and manage your infrastructure nodes
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="mb-6 flex items-center gap-4">
              <div className="relative flex-1 max-w-sm">
                <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search nodes..."
                  className="pl-8"
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                />
              </div>
            </div>

            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Node Name</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Location</TableHead>
                    <TableHead>VMs</TableHead>
                    <TableHead>CPU Allocation</TableHead>
                    <TableHead>RAM Allocation</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={7} className="h-24 text-center">
                        <div className="flex justify-center">
                          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                        </div>
                      </TableCell>
                    </TableRow>
                  ) : filteredNodes.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7} className="h-24 text-center">
                        No nodes found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredNodes.map((node) => (
                      <TableRow key={node.id}>
                        <TableCell>
                          <div className="font-medium">{node.name}</div>
                          <div className="text-xs text-muted-foreground">
                            {node.hostname}
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant={getStatusBadgeVariant(node.status) as React.ComponentProps<typeof Badge>["variant"]} className="capitalize">
                            {node.status}
                          </Badge>
                        </TableCell>
                        <TableCell>{node.location}</TableCell>
                        <TableCell>
                          <div className="flex items-center gap-1">
                            <Server className="h-3 w-3 text-muted-foreground" />
                            <span>{node.vm_count}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="w-full max-w-[120px] space-y-1">
                            <div className="flex items-center justify-between text-xs">
                              <span>
                                {node.cpu_allocated} / {node.cpu_total} Cores
                              </span>
                              <span>
                                {node.cpu_total > 0 ? Math.round(
                                  (node.cpu_allocated / node.cpu_total) * 100
                                ) : 0}
                                %
                              </span>
                            </div>
                            <div className="h-1.5 w-full overflow-hidden rounded-full bg-secondary">
                              <div
                                className="h-full bg-primary"
                                style={{
                                  width: `${node.cpu_total > 0 ? Math.round(
                                    (node.cpu_allocated / node.cpu_total) * 100
                                  ) : 0}%`,
                                }}
                              />
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="w-full max-w-[120px] space-y-1">
                            <div className="flex items-center justify-between text-xs">
                              <span>
                                {node.memory_allocated_gb} /{" "}
                                {node.memory_total_gb} GB
                              </span>
                              <span>
                                {node.memory_total_gb > 0 ? Math.round(
                                  (node.memory_allocated_gb /
                                    node.memory_total_gb) *
                                    100
                                ) : 0}
                                %
                              </span>
                            </div>
                            <div className="h-1.5 w-full overflow-hidden rounded-full bg-secondary">
                              <div
                                className="h-full bg-primary"
                                style={{
                                  width: `${node.memory_total_gb > 0 ? Math.round(
                                    (node.memory_allocated_gb /
                                      node.memory_total_gb) *
                                      100
                                  ) : 0}%`,
                                }}
                              />
                            </div>
                          </div>
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleView(node)}
                              disabled={loadingId === node.id}
                              title="View Details"
                            >
                              {loadingId === node.id ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <Eye className="h-4 w-4" />
                              )}
                              <span className="sr-only">View Details</span>
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleEdit(node)}
                              disabled={loadingId === node.id}
                              title="Edit Node"
                            >
                              {loadingId === node.id ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <Pencil className="h-4 w-4" />
                              )}
                              <span className="sr-only">Edit Node</span>
                            </Button>
                            {node.status === "online" && (
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => openConfirmDialog(node, "drain")}
                                disabled={loadingId === node.id}
                                title="Drain Node"
                              >
                                <ArrowDownToLine className="h-4 w-4 text-warning" />
                                <span className="sr-only">Drain Node</span>
                              </Button>
                            )}
                            {(node.status === "offline" ||
                              node.status === "failed") && (
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() =>
                                  openConfirmDialog(node, "failover")
                                }
                                disabled={loadingId === node.id}
                                title="Initiate Failover"
                              >
                                <RefreshCcw className="h-4 w-4 text-destructive" />
                                <span className="sr-only">Initiate Failover</span>
                              </Button>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>

        {/* Drain/Failover Confirmation Dialog */}
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>
                {dialogAction === "drain" ? "Drain Node" : "Initiate Failover"}
              </DialogTitle>
              <DialogDescription>
                {dialogAction === "drain" ? (
                  <>
                    Are you sure you want to drain <strong>{selectedNode?.name}</strong>?
                    This will migrate all {selectedNode?.vm_count} VMs to other available nodes.
                    This process may take several minutes depending on VM sizes.
                  </>
                ) : (
                  <>
                    Are you sure you want to initiate failover for <strong>{selectedNode?.name}</strong>?
                    This should only be done if the physical node is unrecoverable.
                    All {selectedNode?.vm_count} VMs will be restarted on healthy nodes from their last known state.
                  </>
                )}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setDialogOpen(false)}>
                Cancel
              </Button>
              <Button
                variant={dialogAction === "failover" ? "destructive" : "default"}
                onClick={handleConfirmAction}
              >
                Confirm {dialogAction === "drain" ? "Drain" : "Failover"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Create Node Dialog */}
        <NodeCreateDialog
          open={createDialogOpen}
          onOpenChange={setCreateDialogOpen}
          onCreate={handleCreate}
          isCreating={isCreating}
        />

        {/* Edit Node Dialog */}
        <NodeEditDialog
          open={editDialogOpen}
          onOpenChange={setEditDialogOpen}
          node={editingNode}
          onSave={handleSave}
          isSaving={isSaving}
        />

        {/* Node Detail Dialog (View Only) */}
        <NodeDetailDialog
          open={detailDialogOpen}
          onOpenChange={setDetailDialogOpen}
          node={editingNode}
        />
      </div>
    </div>
  );
}