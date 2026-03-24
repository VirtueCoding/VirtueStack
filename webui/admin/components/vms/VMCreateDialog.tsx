"use client";

import { useEffect, useState, useMemo } from "react";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Server, User, Package, FileCode, Lock, MapPin, Loader2 } from "lucide-react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useToast } from "@/components/ui/use-toast";
import { Badge } from "@/components/ui/badge";
import { adminCustomersApi, adminPlansApi, adminTemplatesApi, adminNodesApi, adminStorageBackendsApi, type Customer, type Plan, type Template, type Node, type StorageBackend } from "@/lib/api-client";

// RFC 1123 hostname validation: lowercase alphanumeric with hyphens, max 63 chars
const hostnameRegex = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/;

// HealthBadge displays storage backend health status as a colored badge
function getHealthBadgeVariant(status: string): "success" | "warning" | "destructive" | "secondary" {
  switch (status) {
    case "healthy":
      return "success";
    case "warning":
      return "warning";
    case "critical":
      return "destructive";
    default:
      return "secondary";
  }
}

function HealthBadge({ status }: { status: string }) {
  return (
    <Badge variant={getHealthBadgeVariant(status)} className="capitalize text-xs">
      {status}
    </Badge>
  );
}

export const createVMSchema = z.object({
  customer_id: z.string().uuid("Must select a valid customer"),
  plan_id: z.string().uuid("Must select a valid plan"),
  template_id: z.string().uuid("Must select a valid template"),
  hostname: z.string()
    .min(1, "Hostname is required")
    .max(63, "Hostname must be 63 characters or less")
    .regex(hostnameRegex, "Hostname must be lowercase alphanumeric with hyphens, starting and ending with alphanumeric"),
  password: z.string()
    .min(12, "Password must be at least 12 characters")
    .max(128, "Password must be 128 characters or less"),
  ssh_keys: z.array(z.string().max(4096)).max(10).optional(),
  location_id: z.string().uuid().optional().or(z.literal("")),
  node_id: z.string().uuid().optional().or(z.literal("")),
});

export type CreateVMFormData = z.infer<typeof createVMSchema>;

interface VMCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (data: CreateVMFormData) => Promise<void>;
  isCreating: boolean;
}

const defaultValues: CreateVMFormData = {
  customer_id: "",
  plan_id: "",
  template_id: "",
  hostname: "",
  password: "",
  location_id: "",
  node_id: "",
};

export function VMCreateDialog({ open, onOpenChange, onCreate, isCreating }: VMCreateDialogProps) {
  const { toast } = useToast();
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [plans, setPlans] = useState<Plan[]>([]);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [storageBackends, setStorageBackends] = useState<StorageBackend[]>([]);
  const [loadingLookups, setLoadingLookups] = useState(true);

  const form = useForm<CreateVMFormData>({
    resolver: zodResolver(createVMSchema),
    defaultValues,
  });

  // Fetch lookup data when dialog opens
  useEffect(() => {
    if (open) {
      form.reset(defaultValues);
      setLoadingLookups(true);
      Promise.all([
        adminCustomersApi.getCustomers(),
        adminPlansApi.getPlans(),
        adminTemplatesApi.getTemplates(),
        adminNodesApi.getNodes(),
        adminStorageBackendsApi.getStorageBackends(),
      ])
        .then(([customersResponse, plansData, templatesData, nodesResponse, backendsData]) => {
          setCustomers(customersResponse.data || []);
          setPlans((plansData || []).filter(p => p.is_active));
          setTemplates(templatesData || []);
          setNodes((nodesResponse.data || []));
          setStorageBackends(backendsData || []);
        })
        .catch(() => {
          toast({
            title: "Error",
            description: "Failed to load required data.",
            variant: "destructive",
          });
        })
        .finally(() => setLoadingLookups(false));
    }
  }, [open, form, toast]);

  // Filter nodes to those with a storage backend of the plan's type
  const selectedPlanId = form.watch("plan_id");
  const selectedPlan = plans.find(p => p.id === selectedPlanId);

  const filteredNodes = useMemo(() => {
    if (!nodes.length || !storageBackends.length || !selectedPlan) {
      return nodes.filter(n => n.status === "online");
    }

    // Get node IDs that have a storage backend of the required type
    const nodeIdsWithType = new Set(
      storageBackends
        .filter(sb => sb.type === selectedPlan.storage_backend && sb.health_status !== "critical")
        .flatMap(sb => sb.nodes?.map(n => n.node_id) || [])
    );

    return nodes.filter(n => n.status === "online" && nodeIdsWithType.has(n.id));
  }, [nodes, storageBackends, selectedPlan]);

  // Get health status for a node from storage backends
  const getNodeHealthStatus = (nodeId: string): "healthy" | "warning" | "critical" | "unknown" => {
    if (!selectedPlan || !storageBackends.length) return "unknown";
    const matchingBackend = storageBackends.find(
      sb => sb.type === selectedPlan.storage_backend && sb.nodes?.some(n => n.node_id === nodeId)
    );
    return matchingBackend?.health_status || "unknown";
  };

  const handleSubmit = async (data: CreateVMFormData) => {
    try {
      // Clean up empty optional fields
      const cleanData = {
        ...data,
        location_id: data.location_id || undefined,
        node_id: data.node_id || undefined,
      };
      await onCreate(cleanData as CreateVMFormData);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Creation Failed",
        description: error instanceof Error ? error.message : "Failed to create VM",
        variant: "destructive",
      });
    }
  };

  // Generate a cryptographically secure random password using the Web Crypto API
  const generatePassword = () => {
    const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*";
    const randomValues = new Uint32Array(16);
    crypto.getRandomValues(randomValues);
    const password = Array.from(randomValues)
      .map((v) => chars.charAt(v % chars.length))
      .join("");
    form.setValue("password", password);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Server className="h-5 w-5" />
            Create Virtual Machine
          </DialogTitle>
          <DialogDescription>
            Create a new VM manually. This is rarely needed as VMs are typically created via billing integration.
          </DialogDescription>
        </DialogHeader>

        {loadingLookups ? (
          <div className="flex justify-center py-8">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-6 py-4">
            {/* Customer Selection */}
            <div className="space-y-2">
              <Label htmlFor="customer_id" className="flex items-center gap-2">
                <User className="h-4 w-4 text-muted-foreground" />
                Customer <span className="text-destructive">*</span>
              </Label>
              <Select
                value={form.watch("customer_id")}
                onValueChange={(value) => form.setValue("customer_id", value)}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select a customer" />
                </SelectTrigger>
                <SelectContent>
                  {customers.map((customer) => (
                    <SelectItem key={customer.id} value={customer.id}>
                      {customer.name} ({customer.email})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {form.formState.errors.customer_id && (
                <p className="text-xs text-destructive">{form.formState.errors.customer_id.message}</p>
              )}
            </div>

            {/* Plan and Template Selection */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="plan_id" className="flex items-center gap-2">
                  <Package className="h-4 w-4 text-muted-foreground" />
                  Plan <span className="text-destructive">*</span>
                </Label>
                <Select
                  value={form.watch("plan_id")}
                  onValueChange={(value) => form.setValue("plan_id", value)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select a plan" />
                  </SelectTrigger>
                  <SelectContent>
                    {plans.map((plan) => (
                      <SelectItem key={plan.id} value={plan.id}>
                        {plan.name} ({plan.vcpu} vCPU, {Math.round(plan.memory_mb / 1024)}GB RAM)
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {form.formState.errors.plan_id && (
                  <p className="text-xs text-destructive">{form.formState.errors.plan_id.message}</p>
                )}
              </div>

              <div className="space-y-2">
                <Label htmlFor="template_id" className="flex items-center gap-2">
                  <FileCode className="h-4 w-4 text-muted-foreground" />
                  Template <span className="text-destructive">*</span>
                </Label>
                <Select
                  value={form.watch("template_id")}
                  onValueChange={(value) => form.setValue("template_id", value)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select a template" />
                  </SelectTrigger>
                  <SelectContent>
                    {templates.map((template) => (
                      <SelectItem key={template.id} value={template.id}>
                        {template.name} ({template.os_family})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {form.formState.errors.template_id && (
                  <p className="text-xs text-destructive">{form.formState.errors.template_id.message}</p>
                )}
              </div>
            </div>

            {/* Hostname */}
            <div className="space-y-2">
              <Label htmlFor="hostname" className="flex items-center gap-2">
                <Server className="h-4 w-4 text-muted-foreground" />
                Hostname <span className="text-destructive">*</span>
              </Label>
              <Input
                id="hostname"
                placeholder="e.g., my-server-01"
                {...form.register("hostname")}
                onChange={(e) => {
                  // Convert to lowercase automatically
                  form.setValue("hostname", e.target.value.toLowerCase());
                }}
              />
              {form.formState.errors.hostname && (
                <p className="text-xs text-destructive">{form.formState.errors.hostname.message}</p>
              )}
              <p className="text-xs text-muted-foreground">Lowercase alphanumeric with hyphens</p>
            </div>

            {/* Password */}
            <div className="space-y-2">
              <Label htmlFor="password" className="flex items-center gap-2">
                <Lock className="h-4 w-4 text-muted-foreground" />
                Root Password <span className="text-destructive">*</span>
              </Label>
              <div className="flex gap-2">
                <Input
                  id="password"
                  type="text"
                  placeholder="Min 12 characters"
                  {...form.register("password")}
                  className="flex-1"
                />
                <Button
                  type="button"
                  variant="outline"
                  onClick={generatePassword}
                  title="Generate random password"
                >
                  Generate
                </Button>
              </div>
              {form.formState.errors.password && (
                <p className="text-xs text-destructive">{form.formState.errors.password.message}</p>
              )}
            </div>

            {/* Location and Node (optional) */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="location_id" className="flex items-center gap-2">
                  <MapPin className="h-4 w-4 text-muted-foreground" />
                  Location (Optional)
                </Label>
                <Select
                  value={form.watch("location_id") || ""}
                  onValueChange={(value) => form.setValue("location_id", value)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Auto-assign" />
                  </SelectTrigger>
                  <SelectContent>
                    {Array.from(new Set(nodes.map(n => n.location))).map((loc) => (
                      <SelectItem key={loc} value={loc}>
                        {loc}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">Leave empty for auto-assignment</p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="node_id" className="flex items-center gap-2">
                  <Server className="h-4 w-4 text-muted-foreground" />
                  Node (Optional)
                </Label>
                <Select
                  value={form.watch("node_id") || ""}
                  onValueChange={(value) => form.setValue("node_id", value)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={selectedPlan ? "Auto-assign" : "Select a plan first"} />
                  </SelectTrigger>
                  <SelectContent>
                    {filteredNodes.map((node) => {
                      const health = getNodeHealthStatus(node.id);
                      return (
                        <SelectItem key={node.id} value={node.id} className="flex items-center gap-2">
                          <span>{node.name} ({node.location})</span>
                          <HealthBadge status={health} />
                        </SelectItem>
                      );
                    })}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  {selectedPlan
                    ? `Only showing nodes with ${selectedPlan.storage_backend.toUpperCase()} storage`
                    : "Select a plan to see compatible nodes"}
                </p>
              </div>
            </div>

            <DialogFooter className="pt-4">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={isCreating}>
                Cancel
              </Button>
              <Button type="submit" disabled={isCreating}>
                {isCreating && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Create VM
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}