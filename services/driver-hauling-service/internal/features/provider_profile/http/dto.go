package providerprofilehttp

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
	LicenseExpiryYear            string `json:"license_expiry_year"`
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
	SubmitForVerification        bool   `json:"submit_for_verification"`
}

type createTruckRequest struct {
	TruckType         string   `json:"truck_type"`
	CapacityKg        int      `json:"capacity_kg"`
	PlateNumber       string   `json:"plate_number"`
	Year              *int     `json:"year"`
	Make              *string  `json:"make"`
	Model             *string  `json:"model"`
	Color             *string  `json:"color"`
	LicenseType       string   `json:"license_type"`
	NumberOfAxles     string   `json:"number_of_axles"`
	YearsOfExperience string   `json:"years_of_experience"`
	GoodsTypes        []string `json:"goods_types"`
	HasInsurance      bool     `json:"has_insurance"`
}

type updateTruckRequest struct {
	TruckType         string   `json:"truck_type"`
	CapacityKg        int      `json:"capacity_kg"`
	PlateNumber       string   `json:"plate_number"`
	Year              *int     `json:"year"`
	Make              *string  `json:"make"`
	Model             *string  `json:"model"`
	Color             *string  `json:"color"`
	LicenseType       string   `json:"license_type"`
	NumberOfAxles     string   `json:"number_of_axles"`
	YearsOfExperience string   `json:"years_of_experience"`
	GoodsTypes        []string `json:"goods_types"`
	HasInsurance      bool     `json:"has_insurance"`
	Status            string   `json:"status"`
}
