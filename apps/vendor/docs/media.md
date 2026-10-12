# Vendor media uploads and access

Implemented 11 October 2026. Mock remains the default. Live uses Vendor Service for staff/tenant/record authorization, Media File Service for private bytes and validation, then the existing version-checked owner attachment write.

## App entry points

- Catalogue -> Food item -> Manage photos. Add/remove images, choose a primary photo, preview and save the ordered reference list. Maximum 12 images, matching Food.
- Catalogue -> Shop product -> Manage photos. Add/remove, preview and save media_refs without replacing other product fields. Maximum 12 images, matching Shop.
- More -> Brands -> brand details -> Manage brand images. Choose LOGO, COVER or GALLERY; preserve name/description and other images. Saving follows the backend's existing verification invalidation rules.
- Pastry -> Custom request -> Design/specification -> Reference media. Upload images/PDFs, preview photos, save PDFs, then Use references. The owning design/specification/change save still commits these references; an upload alone does not save the design. Maximum 20 references, matching Pastry.

Images accept JPEG, PNG or WebP, up to 10 MB each. PDF is allowed only for pastry references. New items/brands/requests must be saved before files can be uploaded because authorization binds the existing record ID. Legacy references without the new purpose/record binding remain preserved but cannot be previewed through this new bridge. Their original access integration is not assumed.

## Backend contracts

POST /api/v1/vendor/media/uploads accepts target_type, target_ref, file_name, mime_type and byte_size with Idempotency-Key. Only FOOD_ITEM, SHOP_PRODUCT, BRAND and PASTRY_REQUEST are accepted. Feature/write flags, real staff permissions, actual ownership and branch scope are checked; brand editing requires business-wide access. Media private grants use PRIVATE visibility, VENDOR_BUSINESS ownership, purpose PRODUCT_IMAGE / BRAND_IMAGE / PASTRY_REFERENCE, and ASSET binding target_type:target_ref.

GET /api/v1/vendor/media/objects/{mediaRef} requires target_type and target_ref, returns actual upload/safety status and usable. GET the /content child returns verified bounded image/PDF bytes through server-only private download grants. No public image URL, service secret or storage object key is exposed to the app. New product/brand/pastry attachments must be SAFE and bound to their owner record; legacy references are preserved by existing attachment semantics.

Existing attachment operations remain authoritative: Food PUT /food/items/{itemId}/media; Shop PATCH /shop/products/{productId}; Brand PATCH /businesses/{businessId}/brands/{brandId}; Pastry specification/change writes. The Food generated request was corrected to its real media array wrapper rather than flattened anonymous-struct fields.

## Recovery and UI states

Preview bytes and upload/download tokens remain in memory. Durable scoped upload metadata contains request identity, checksum, original authorization key and issued media reference only. Interrupted uploads check status first; incomplete uploads require the same file; expired/rejected uploads can be discarded. Completed-but-unverified files cannot be attached. Attachment retries retain the frozen body, expected version and original key, rather than creating a new command after an unknown response. Unsaved gallery references are stored separately by mode/origin/business/staff/target. Concurrent owner changes are refused rather than overwritten. Removing a reference detaches it from the record; it does not delete Media storage.

Mock runs the same grant/upload/read/save sequence with actual sample bytes, file signatures/checksums, target-bound access and optimistic versions. Mock files and grants are fixtures in the current Mock transport session; reconnecting resets that transport and requires fresh fixture uploads. Live credentials are never used in Mock.

## Live deployment

Deploy both changed services and apply media-file-service/migrations/000006_vendor_asset_privacy.up.sql. Vendor needs VENDOR_MEDIA_URL, VENDOR_MEDIA_PUBLIC_URL and VENDOR_MEDIA_SECRET. Media needs its private API, vendor-service caller/secret configuration, private storage, download signing key and JPEG/PNG/WebP/PDF MIME policy. The public Media origin must be reachable from the phone.

Media now supports an optional ClamAV daemon through MEDIA_FILE_SCANNER_ADDRESS, MEDIA_FILE_SCANNER_NETWORK=tcp|unix, and MEDIA_FILE_SCANNER_TIMEOUT_SECONDS (default 5). Keep the scanner inside the trusted service network, maintain its signature database, and configure StreamMaxLength/scan limits for the permitted upload size. Protocol reference: https://docs.clamav.net/manual/Usage/ClamdProtocol.html. Blank/unreachable/erroring scanner leaves files unverified; only a complete clean verdict produces SAFE. Deploying code does not install or run the external scanner. Existing unverified files need a new upload after scanner setup; no public rescan workflow was added.

Focused Mock upload/UI, Media scope/policy and scanner-protocol checks are development evidence. PostgreSQL migrations, real scanner/storage, deployed permissions, lost-response recovery and physical-phone checks still require final Live verification.
