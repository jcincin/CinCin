"use client";

import { useState, useEffect, useCallback, Suspense } from "react";
import { UserButton, useUser } from "@clerk/nextjs";
import { useQuery } from "convex/react";
import { useSearchParams } from "next/navigation";
import { api } from "@/convex/_generated/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { ReservationCard, type Reservation } from "@/components/reservation-card";
import { ReservationDetailDialog } from "@/components/reservation-detail-dialog";
import { ResyLinkModal } from "@/components/resy-link-modal";

const RESTAURANTS = [
  { venue_id: 86907, name: "Crevette" },
  { venue_id: 82039, name: "BeefBar" },
  { venue_id: 70599, name: "Forgione" },
];

const TABLE_TYPES = [
  { value: "dining", label: "Dining Room" },
  { value: "indoor", label: "Indoor" },
  { value: "outdoor", label: "Outdoor" },
  { value: "patio", label: "Patio" },
  { value: "bar", label: "Bar" },
  { value: "lounge", label: "Lounge" },
  { value: "booth", label: "Booth" },
];

function HomeContent() {
  const { user: clerkUser } = useUser();
  const searchParams = useSearchParams();

  // Get Convex user and usage data
  const convexUser = useQuery(
    api.users.getUserByClerkId,
    clerkUser?.id ? { clerkId: clerkUser.id } : "skip"
  );
  const usageData = useQuery(
    api.usage.getUsageWithLimits,
    convexUser?._id ? { userId: convexUser._id } : "skip"
  );

  // Check for checkout success
  const checkoutStatus = searchParams.get("checkout");
  const [showWelcome, setShowWelcome] = useState(checkoutStatus === "success");

  // Restaurant selection
  const [selectedVenueId, setSelectedVenueId] = useState<number | null>(null);

  // Reservation form state
  const [reservationDate, setReservationDate] = useState("");
  const [reservationHour, setReservationHour] = useState("7");
  const [reservationMinute, setReservationMinute] = useState("00");
  const [reservationPeriod, setReservationPeriod] = useState<"AM" | "PM">("PM");
  const [tablePreferences, setTablePreferences] = useState<string[]>([]);
  const [scheduleMode, setScheduleMode] = useState<"immediate" | "auto" | "manual">("immediate");
  const [scheduledDate, setScheduledDate] = useState("");
  const [scheduledTime, setScheduledTime] = useState("");

  // UI state
  const [isReserving, setIsReserving] = useState(false);
  const [message, setMessage] = useState<{ type: "success" | "error"; text: string } | null>(null);
  const [logs, setLogs] = useState<string[]>([]);

  // Reservations state
  const [reservations, setReservations] = useState<Reservation[]>([]);
  const [isLoadingReservations, setIsLoadingReservations] = useState(true);
  const [cancellingId, setCancellingId] = useState<string | null>(null);
  const [selectedReservation, setSelectedReservation] = useState<Reservation | null>(null);
  const [isDetailDialogOpen, setIsDetailDialogOpen] = useState(false);

  // Resy link state
  const [isResyLinked, setIsResyLinked] = useState<boolean | null>(null);
  const [isResyLinkModalOpen, setIsResyLinkModalOpen] = useState(false);

  // Check Resy link status on mount
  const checkResyLinkStatus = useCallback(async () => {
    try {
      const res = await fetch("/api/resy/status");
      if (res.ok) {
        const data = await res.json();
        setIsResyLinked(data.linked);
      }
    } catch {
      // Ignore errors
    }
  }, []);

  useEffect(() => {
    checkResyLinkStatus();
  }, [checkResyLinkStatus]);

  // Fetch logs periodically
  useEffect(() => {
    const fetchLogs = async () => {
      try {
        const res = await fetch("/api/logs");
        if (res.ok) {
          const data = await res.json();
          setLogs(data.slice(-10)); // Last 10 logs
        }
      } catch {
        // Ignore errors
      }
    };

    fetchLogs();
    const interval = setInterval(fetchLogs, 5000);
    return () => clearInterval(interval);
  }, []);

  // Fetch reservations function
  const fetchReservations = useCallback(async () => {
    try {
      const res = await fetch("/api/reservations");
      if (res.ok) {
        const data = await res.json();
        setReservations(data.reservations || []);
      }
    } catch {
      // Ignore errors
    } finally {
      setIsLoadingReservations(false);
    }
  }, []);

  // Fetch reservations on mount and periodically
  useEffect(() => {
    fetchReservations();
    const interval = setInterval(fetchReservations, 10000);
    return () => clearInterval(interval);
  }, [fetchReservations]);

  // Cancel reservation handler
  const handleCancelReservation = async (id: string) => {
    setCancellingId(id);
    try {
      const res = await fetch(`/api/reservations/${id}`, { method: "DELETE" });
      if (res.ok) {
        setReservations((prev) => prev.filter((r) => r.id !== id));
        setMessage({ type: "success", text: "Reservation cancelled" });
      } else {
        const data = await res.json();
        setMessage({ type: "error", text: data.error || "Failed to cancel" });
      }
    } catch {
      setMessage({ type: "error", text: "Failed to cancel reservation" });
    } finally {
      setCancellingId(null);
    }
  };

  const toggleTablePreference = (value: string) => {
    setTablePreferences((prev) =>
      prev.includes(value) ? prev.filter((p) => p !== value) : [...prev, value]
    );
  };

  const handleReserve = async () => {
    if (!selectedVenueId) {
      setMessage({ type: "error", text: "Please select a restaurant first" });
      return;
    }

    if (!reservationDate) {
      setMessage({ type: "error", text: "Please select a date" });
      return;
    }

    if (scheduleMode === "manual" && (!scheduledDate || !scheduledTime)) {
      setMessage({ type: "error", text: "Please set when to attempt the reservation" });
      return;
    }

    setIsReserving(true);
    setMessage(null);

    // Convert 12-hour to 24-hour format
    let hour24 = parseInt(reservationHour);
    if (reservationPeriod === "AM" && hour24 === 12) {
      hour24 = 0;
    } else if (reservationPeriod === "PM" && hour24 !== 12) {
      hour24 += 12;
    }
    const reservationTime = `${hour24.toString().padStart(2, "0")}:${reservationMinute}`;
    const reservationDateTime = `${reservationDate}T${reservationTime}`;
    const requestDateTime = scheduleMode === "manual" ? `${scheduledDate}T${scheduledTime}` : "";

    try {
      const res = await fetch("/api/reserve", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          venue_id: selectedVenueId,
          reservation_time: reservationDateTime,
          party_size: 2,
          table_preferences: tablePreferences,
          is_immediate: scheduleMode === "immediate",
          auto_schedule: scheduleMode === "auto",
          request_time: requestDateTime,
        }),
      });

      const data = await res.json();

      if (data.error) {
        setMessage({ type: "error", text: data.error });
      } else if (data.reservation_time) {
        setMessage({ type: "success", text: `Reserved for ${data.reservation_time}` });
      } else if (data.reservation_id) {
        // Refresh reservations list to show the new one
        await fetchReservations();
        setMessage({ type: "success", text: "Reservation scheduled!" });
      }
    } catch {
      setMessage({ type: "error", text: "Failed to make reservation" });
    } finally {
      setIsReserving(false);
    }
  };

  return (
    <>
      <style>{`
        input[type="date"]::-webkit-calendar-picker-indicator,
        input[type="time"]::-webkit-calendar-picker-indicator {
          filter: invert(1);
          cursor: pointer;
        }
        
        input[type="date"]::-moz-calendar-picker-indicator,
        input[type="time"]::-moz-calendar-picker-indicator {
          filter: invert(1);
          cursor: pointer;
        }
      `}</style>
      <div className="min-h-screen bg-black text-white relative overflow-hidden">
        {/* Corner brackets */}
      <div className="absolute top-8 left-8 w-16 h-16 border-l-2 border-t-2 border-amber-400" />
      <div className="absolute top-8 right-8 w-16 h-16 border-r-2 border-t-2 border-amber-400" />
      <div className="absolute bottom-8 left-8 w-16 h-16 border-l-2 border-b-2 border-amber-400" />
      <div className="absolute bottom-8 right-8 w-16 h-16 border-r-2 border-b-2 border-amber-400" />

      {/* Header */}
      <div className="flex items-center justify-between px-8 pt-8 pb-4">
        <div className="w-10" /> {/* Spacer for centering */}
        <h1 className="font-serif text-4xl font-bold bg-linear-to-b from-gray-200 to-gray-500 bg-clip-text text-transparent tracking-wide">
          CIN CIN
        </h1>
        <UserButton
          appearance={{
            elements: {
              avatarBox: "w-10 h-10",
            },
          }}
        />
      </div>

      {/* Usage Display */}
      {usageData && (
        <div className="max-w-xl mx-auto px-8 mb-4">
          <div className="bg-zinc-900/50 border border-zinc-800 rounded-lg p-4">
            <div className="flex items-center justify-between mb-3">
              <span className="text-zinc-400 text-sm">Monthly Usage</span>
              <span className="text-amber-400 text-xs font-medium uppercase tracking-wider">
                {usageData.tier} Plan
              </span>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <div className="flex justify-between text-xs mb-1">
                  <span className="text-zinc-500">Reservations</span>
                  <span className="text-zinc-300">
                    {usageData.reservations.used}/{usageData.reservations.limit}
                  </span>
                </div>
                <div className="h-1.5 bg-zinc-800 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-amber-400 rounded-full transition-all"
                    style={{
                      width: `${Math.min(100, (usageData.reservations.used / usageData.reservations.limit) * 100)}%`,
                    }}
                  />
                </div>
              </div>
              <div>
                <div className="flex justify-between text-xs mb-1">
                  <span className="text-zinc-500">Concierge</span>
                  <span className="text-zinc-300">
                    {usageData.conciergeBookings.used}/{usageData.conciergeBookings.limit}
                  </span>
                </div>
                <div className="h-1.5 bg-zinc-800 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-amber-400 rounded-full transition-all"
                    style={{
                      width: `${Math.min(100, (usageData.conciergeBookings.used / usageData.conciergeBookings.limit) * 100)}%`,
                    }}
                  />
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Welcome Message (after successful checkout) */}
      {showWelcome && (
        <div className="max-w-xl mx-auto px-8 mb-4">
          <div className="bg-green-500/10 border border-green-500/30 rounded-lg p-4 flex items-center justify-between">
            <div>
              <p className="text-green-400 font-medium">Welcome to Cin Cin!</p>
              <p className="text-zinc-400 text-sm">Your membership is now active</p>
            </div>
            <button
              onClick={() => setShowWelcome(false)}
              className="text-zinc-500 hover:text-zinc-300"
            >
              <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      )}

      {/* Resy Link Banner */}
      {isResyLinked === false && (
        <div className="max-w-xl mx-auto px-8 mb-4">
          <div className="bg-amber-400/10 border border-amber-400/30 rounded-lg p-4 flex items-center justify-between">
            <div>
              <p className="text-amber-400 font-medium">Link Your Resy Account</p>
              <p className="text-zinc-400 text-sm">Connect your Resy account to make reservations</p>
            </div>
            <Button
              onClick={() => setIsResyLinkModalOpen(true)}
              className="bg-amber-400 text-black hover:bg-amber-500"
            >
              Link Account
            </Button>
          </div>
        </div>
      )}

      {/* Main content */}
      <div className="max-w-xl mx-auto px-8 pb-24">
        <Card className="bg-zinc-900/50 border-zinc-800">
          <CardHeader>
            <CardTitle className="text-amber-400 text-2xl font-light tracking-wider text-center">
              BOOK RESERVATION
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            {/* Message */}
            {message && (
              <div
                className={`p-4 rounded-lg text-center ${
                  message.type === "success"
                    ? "bg-green-500/10 border border-green-500/30 text-green-400"
                    : "bg-red-500/10 border border-red-500/30 text-red-400"
                }`}
              >
                {message.text}
              </div>
            )}

            {/* Restaurant Selection */}
            <div className="space-y-2">
              <Label className="text-zinc-400 text-sm">Restaurant</Label>
              <div className="grid grid-cols-3 gap-2">
                {RESTAURANTS.map((restaurant) => (
                  <button
                    key={restaurant.venue_id}
                    onClick={() => setSelectedVenueId(restaurant.venue_id)}
                    className={`py-3 px-2 rounded-lg text-sm font-medium transition-colors ${
                      selectedVenueId === restaurant.venue_id
                        ? "bg-amber-400 text-black"
                        : "bg-zinc-800 text-zinc-400 hover:bg-zinc-700"
                    }`}
                  >
                    {restaurant.name}
                  </button>
                ))}
              </div>
            </div>

            {/* Date & Time */}
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className="text-zinc-400 text-sm">Reservation Date</Label>
                <Input
                  type="date"
                  value={reservationDate}
                  onChange={(e) => setReservationDate(e.target.value)}
                  className="bg-zinc-800 border-zinc-700 text-white"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-zinc-400 text-sm">Reservation Time</Label>
                <div className="flex gap-2">
                  <select
                    value={reservationHour}
                    onChange={(e) => setReservationHour(e.target.value)}
                    className="flex-1 h-10 rounded-md bg-zinc-800 border border-zinc-700 text-white px-3 text-sm appearance-none cursor-pointer"
                  >
                    {[12, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11].map((h) => (
                      <option key={h} value={h}>{h}</option>
                    ))}
                  </select>
                  <select
                    value={reservationMinute}
                    onChange={(e) => setReservationMinute(e.target.value)}
                    className="flex-1 h-10 rounded-md bg-zinc-800 border border-zinc-700 text-white px-3 text-sm appearance-none cursor-pointer"
                  >
                    {["00", "15", "30", "45"].map((m) => (
                      <option key={m} value={m}>{m}</option>
                    ))}
                  </select>
                  <select
                    value={reservationPeriod}
                    onChange={(e) => setReservationPeriod(e.target.value as "AM" | "PM")}
                    className="w-20 h-10 rounded-md bg-zinc-800 border border-zinc-700 text-white px-3 text-sm appearance-none cursor-pointer"
                  >
                    <option value="AM">AM</option>
                    <option value="PM">PM</option>
                  </select>
                </div>
              </div>
            </div>

            {/* Party Size (hardcoded display) */}
            <div className="flex items-center justify-between p-3 bg-zinc-800/50 rounded-lg">
              <span className="text-zinc-400 text-sm">Party Size</span>
              <span className="text-white font-medium">2 guests</span>
            </div>

            {/* Table Preferences */}
            {/* <div className="space-y-2">
              <Label className="text-zinc-400 text-sm">Table Preferences (Optional)</Label>
              <div className="flex flex-wrap gap-2">
                {TABLE_TYPES.map((type) => (
                  <button
                    key={type.value}
                    onClick={() => toggleTablePreference(type.value)}
                    className={`px-3 py-1.5 rounded-full text-sm transition-colors ${
                      tablePreferences.includes(type.value)
                        ? "bg-amber-400 text-black"
                        : "bg-zinc-800 text-zinc-400 hover:bg-zinc-700"
                    }`}
                  >
                    {type.label}
                  </button>
                ))}
              </div>
            </div> */}

            {/* Timing Toggle */}
            <div className="space-y-4">
              <Label className="text-zinc-400 text-sm">Booking Mode</Label>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setScheduleMode("immediate")}
                  className={`flex-1 py-3 rounded-lg text-sm font-medium transition-colors ${
                    scheduleMode === "immediate"
                      ? "bg-amber-400 text-black"
                      : "bg-zinc-800 text-zinc-400 hover:bg-zinc-700"
                  }`}
                >
                  Now
                </button>
                <button
                  onClick={() => setScheduleMode("auto")}
                  className={`flex-1 py-3 rounded-lg text-sm font-medium transition-colors ${
                    scheduleMode === "auto"
                      ? "bg-amber-400 text-black"
                      : "bg-zinc-800 text-zinc-400 hover:bg-zinc-700"
                  }`}
                >
                  Concierge
                </button>
                <button
                  onClick={() => setScheduleMode("manual")}
                  className={`flex-1 py-3 rounded-lg text-sm font-medium transition-colors ${
                    scheduleMode === "manual"
                      ? "bg-amber-400 text-black"
                      : "bg-zinc-800 text-zinc-400 hover:bg-zinc-700"
                  }`}
                >
                  Manual
                </button>
              </div>

              {/* Auto Schedule Info */}
              {scheduleMode === "auto" && (
                <div className="p-4 bg-zinc-800/50 rounded-lg">
                  <p className="text-xs text-zinc-400">
                    Concierge will detect when reservations open for this restaurant and schedule the booking attempt automatically.
                  </p>
                </div>
              )}

              {/* Manual Schedule (custom date/time) */}
              {scheduleMode === "manual" && (
                <div className="grid grid-cols-2 gap-4 p-4 bg-zinc-800/50 rounded-lg">
                  <div className="space-y-2">
                    <Label className="text-zinc-400 text-sm">Attempt On Date</Label>
                    <Input
                      type="date"
                      value={scheduledDate}
                      onChange={(e) => setScheduledDate(e.target.value)}
                      className="bg-zinc-700 border-zinc-600 text-white"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label className="text-zinc-400 text-sm">Attempt At Time</Label>
                    <Input
                      type="time"
                      value={scheduledTime}
                      onChange={(e) => setScheduledTime(e.target.value)}
                      className="bg-zinc-700 border-zinc-600 text-white"
                    />
                  </div>
                  <p className="col-span-full text-xs text-zinc-500">
                    The bot will attempt to book exactly at this time (NYC timezone)
                  </p>
                </div>
              )}
            </div>

            {/* Reserve Button */}
            <Button
              onClick={handleReserve}
              disabled={isReserving || !selectedVenueId}
              className="w-full py-6 text-lg bg-amber-400 text-black hover:bg-amber-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isReserving
                ? "RESERVING..."
                : scheduleMode === "immediate"
                ? "RESERVE NOW"
                : scheduleMode === "auto"
                ? "SCHEDULE CONCIERGE"
                : "SCHEDULE"}
            </Button>

            {!selectedVenueId && (
              <p className="text-center text-zinc-500 text-sm">
                Select a restaurant to continue
              </p>
            )}
          </CardContent>
        </Card>

        {/* Scheduled Reservations Section */}
        {!isLoadingReservations && reservations.length > 0 && (
          <div className="mt-8">
            <h2 className="text-amber-400 text-xl font-light tracking-wider text-center mb-4">
              SCHEDULED RESERVATIONS
            </h2>
            <div className="space-y-3">
              {reservations.map((reservation) => (
                <ReservationCard
                  key={reservation.id}
                  reservation={reservation}
                  onView={() => {
                    setSelectedReservation(reservation);
                    setIsDetailDialogOpen(true);
                  }}
                  onCancel={() => handleCancelReservation(reservation.id)}
                  isCancelling={cancellingId === reservation.id}
                />
              ))}
            </div>
          </div>
        )}

        {/* Empty state */}
        {!isLoadingReservations && reservations.length === 0 && (
          <div className="mt-8 text-center text-zinc-500 text-sm">
            No scheduled reservations
          </div>
        )}
      </div>

      {/* Reservation Detail Dialog */}
      <ReservationDetailDialog
        reservation={selectedReservation}
        open={isDetailDialogOpen}
        onOpenChange={setIsDetailDialogOpen}
      />

      {/* Resy Link Modal */}
      <ResyLinkModal
        open={isResyLinkModalOpen}
        onOpenChange={setIsResyLinkModalOpen}
        onLinked={() => {
          setIsResyLinked(true);
          checkResyLinkStatus();
        }}
      />
    </div>
    </>
  );
}

export default function Home() {
  return (
    <Suspense
      fallback={
        <div className="min-h-screen bg-black flex items-center justify-center">
          <div className="animate-pulse text-amber-400">Loading...</div>
        </div>
      }
    >
      <HomeContent />
    </Suspense>
  );
}
