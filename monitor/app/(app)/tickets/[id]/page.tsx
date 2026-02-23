"use client";

import { useParams } from "next/navigation";
import { TicketDetail } from "@/components/ticket-detail";

export default function TicketDetailPage() {
  const params = useParams();
  const id = params.id as string;

  return <TicketDetail id={id} />;
}
