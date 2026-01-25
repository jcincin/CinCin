"use client";

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import type { Reservation } from "@/components/reservation-card";

interface ReservationDetailDialogProps {
  reservation: Reservation | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function DetailRow({
  label,
  value,
  className,
}: {
  label: string;
  value: string;
  className?: string;
}) {
  return (
    <div className="flex justify-between items-start gap-4">
      <span className="text-zinc-400 text-sm">{label}</span>
      <span className={className || "text-white text-sm text-right"}>
        {value}
      </span>
    </div>
  );
}

export function ReservationDetailDialog({
  reservation,
  open,
  onOpenChange,
}: ReservationDetailDialogProps) {
  if (!reservation) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="bg-zinc-900 border-zinc-800 text-white">
        <DialogHeader>
          <DialogTitle className="text-amber-400 font-light tracking-wider">
            Reservation Details
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-3 py-2">
          <DetailRow label="Restaurant" value={reservation.venue_name} />
          <DetailRow
            label="Reservation Time"
            value={reservation.reservation_time}
          />
          <DetailRow
            label="Party Size"
            value={`${reservation.party_size} ${reservation.party_size === 1 ? "guest" : "guests"}`}
          />
          <DetailRow label="Sniper Runs" value={reservation.run_time} />
          {reservation.table_preferences &&
            reservation.table_preferences.length > 0 && (
              <DetailRow
                label="Table Preferences"
                value={reservation.table_preferences.join(", ")}
              />
            )}
          <DetailRow label="Created" value={reservation.created_at} />
          <DetailRow
            label="ID"
            value={reservation.id}
            className="text-zinc-500 text-xs text-right font-mono"
          />
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="border-zinc-700 hover:bg-zinc-800"
          >
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
