// Nigeria's 36 states (1999 Constitution, section 3) and the separate FCT.
// Presence here expresses future geographic interest, not operational availability.
export const NIGERIA_STATES = [
  "Abia",
  "Adamawa",
  "Akwa Ibom",
  "Anambra",
  "Bauchi",
  "Bayelsa",
  "Benue",
  "Borno",
  "Cross River",
  "Delta",
  "Ebonyi",
  "Edo",
  "Ekiti",
  "Enugu",
  "Gombe",
  "Imo",
  "Jigawa",
  "Kaduna",
  "Kano",
  "Katsina",
  "Kebbi",
  "Kogi",
  "Kwara",
  "Lagos",
  "Nasarawa",
  "Niger",
  "Ogun",
  "Ondo",
  "Osun",
  "Oyo",
  "Plateau",
  "Rivers",
  "Sokoto",
  "Taraba",
  "Yobe",
  "Zamfara",
] as const;

export const FCT = "Federal Capital Territory (Abuja)" as const;

export const NIGERIA_STATES_AND_FCT = [
  ...NIGERIA_STATES.slice(0, 14),
  FCT,
  ...NIGERIA_STATES.slice(14),
] as const;

// Exactly four featured markets; Abuja refers to the FCT, Port Harcourt to Rivers.
export const FEATURED_LOCATIONS = ["Lagos","Abuja","Port Harcourt","Oyo State"] as const;
