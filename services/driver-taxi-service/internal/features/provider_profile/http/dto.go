package providerprofilehttp

// updateProfileRequest carries whatever the current screen renders. Every
// omitted string field is preserved server-side (see UpdateProfile's
// preserve-on-empty SQL), so a partial submit never blanks another screen's
// fields.
// setParcelPreferenceRequest is its own tiny body rather than a field on
// updateProfileRequest -- see ProfileService.SetAcceptsParcels for why a
// boolean cannot ride the preserve-on-empty profile update.
type setParcelPreferenceRequest struct {
	AcceptsParcels bool `json:"accepts_parcels"`
}

type updateProfileRequest struct {
	FirstName                    string `json:"first_name"`
	LastName                     string `json:"last_name"`
	Email                        string `json:"email"`
	Phone                        string `json:"phone"`
	LocationState                string `json:"location_state"`
	LocationCity                 string `json:"location_city"`
	OperationMode                string `json:"operation_mode"`
	ServiceType                  string `json:"service_type"`
	Language                     string `json:"language"`
	DriverLicenseNumber          string `json:"driver_license_number"`
	LicenseExpiryDate            string `json:"license_expiry_date"`
	GovIDURL                     string `json:"gov_id_url"`
	DriverLicenseURL             string `json:"driver_license_url"`
	VehicleRegURL                string `json:"vehicle_reg_url"`
	GuarantorName                string `json:"guarantor_name"`
	GuarantorPhone               string `json:"guarantor_phone"`
	EmergencyContactName         string `json:"emergency_contact_name"`
	EmergencyContactPhone        string `json:"emergency_contact_phone"`
	EmergencyContactRelationship string `json:"emergency_contact_relationship"`
	ProfilePhotoURL              string `json:"profile_photo_url"`
	PhotoAssetID                 string `json:"photo_asset_id"`
}

type createVehicleRequest struct {
	VehicleClass string  `json:"vehicle_class"`
	PlateNumber  string  `json:"plate_number"`
	Seats        int     `json:"seats"`
	Year         *int    `json:"year"`
	Make         *string `json:"make"`
	Model        *string `json:"model"`
	Color        *string `json:"color"`
}

type updateVehicleRequest struct {
	VehicleClass string  `json:"vehicle_class"`
	PlateNumber  string  `json:"plate_number"`
	Seats        int     `json:"seats"`
	Year         *int    `json:"year"`
	Make         *string `json:"make"`
	Model        *string `json:"model"`
	Color        *string `json:"color"`
	Status       string  `json:"status"`
}
