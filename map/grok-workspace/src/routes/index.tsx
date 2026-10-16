import { createFileRoute } from "@tanstack/react-router";
import { AtlasApp } from "@/components/atlas/AtlasApp";

export const Route = createFileRoute("/")({ component: AtlasApp });
