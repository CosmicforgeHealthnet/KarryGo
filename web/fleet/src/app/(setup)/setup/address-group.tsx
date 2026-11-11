"use client";
import type { BusinessAddress } from "@/lib/services/organizationService";
import { Field } from "./onboarding-ui";
export function normalizeAddress(
  value: BusinessAddress | undefined,
  country: string,
): BusinessAddress {
  return {
    ...value,
    street: String(
      value?.street ?? value?.streetAddress ?? value?.addressLine1 ?? "",
    ),
    city: String(value?.city ?? ""),
    state: String(value?.state ?? ""),
    postalCode: String(value?.postalCode ?? ""),
    country: String(value?.country ?? value?.countryCode ?? country),
  };
}
export function AddressGroup({
  prefix,
  value,
  onChange,
}: {
  prefix: string;
  value: BusinessAddress;
  onChange: (value: BusinessAddress) => void;
}) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {(
        [
          ["street", "Street address"],
          ["city", "City"],
          ["state", "State or province"],
          ["postalCode", "Postal code (where applicable)"],
          ["country", "Country code"],
        ] as const
      ).map(([key, label]) => (
        <Field
          key={key}
          name={`${prefix}-${key}`}
          label={label}
          required={key !== "postalCode"}
          value={String(value[key] ?? "")}
          maxLength={key === "country" ? 2 : 250}
          pattern={key === "country" ? "[A-Z]{2}" : undefined}
          hint={
            key === "country"
              ? "Two-letter country code, e.g. NG, GH or GB."
              : undefined
          }
          onChange={(e) =>
            onChange({
              ...value,
              [key]:
                key === "country"
                  ? e.target.value.toUpperCase()
                  : e.target.value,
            })
          }
        />
      ))}
    </div>
  );
}
