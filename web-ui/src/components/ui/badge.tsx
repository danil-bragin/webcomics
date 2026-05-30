import * as React from "react";
import { cn } from "@/lib/cn";

const variants = {
  default: "bg-secondary text-secondary-foreground",
  success: "bg-green-600/20 text-green-300 border border-green-600/40",
  warning: "bg-amber-500/20 text-amber-300 border border-amber-500/40",
  danger: "bg-red-600/20 text-red-300 border border-red-600/40",
  info: "bg-blue-600/20 text-blue-300 border border-blue-600/40",
} as const;

export function Badge({ className, variant = "default", ...p }: React.HTMLAttributes<HTMLSpanElement> & { variant?: keyof typeof variants }) {
  return <span className={cn("inline-flex items-center rounded px-2 py-0.5 text-xs font-medium", variants[variant], className)} {...p} />;
}
