import { create } from "zustand";

interface ApplyFormState {
  firstName: string;
  lastName: string;
  email: string;
  referredBy: string;
  message: string;
  isSubmitting: boolean;
  isSubmitted: boolean;
  error: string;

  setFirstName: (firstName: string) => void;
  setLastName: (lastName: string) => void;
  setEmail: (email: string) => void;
  setReferredBy: (referredBy: string) => void;
  setMessage: (message: string) => void;
  setIsSubmitting: (isSubmitting: boolean) => void;
  setIsSubmitted: (isSubmitted: boolean) => void;
  setError: (error: string) => void;
  reset: () => void;
}

const initialState = {
  firstName: "",
  lastName: "",
  email: "",
  referredBy: "",
  message: "",
  isSubmitting: false,
  isSubmitted: false,
  error: "",
};

export const useApplyFormStore = create<ApplyFormState>((set) => ({
  ...initialState,
  setFirstName: (firstName) => set({ firstName }),
  setLastName: (lastName) => set({ lastName }),
  setEmail: (email) => set({ email }),
  setReferredBy: (referredBy) => set({ referredBy }),
  setMessage: (message) => set({ message }),
  setIsSubmitting: (isSubmitting) => set({ isSubmitting }),
  setIsSubmitted: (isSubmitted) => set({ isSubmitted }),
  setError: (error) => set({ error }),
  reset: () => set(initialState),
}));
