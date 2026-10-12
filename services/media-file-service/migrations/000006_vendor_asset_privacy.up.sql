ALTER TABLE media_objects ADD CONSTRAINT vendor_asset_private_ck CHECK (upload_purpose NOT IN ('PRODUCT_IMAGE','BRAND_IMAGE','PASTRY_REFERENCE') OR visibility='PRIVATE');
ALTER TABLE media_upload_authorizations ADD CONSTRAINT vendor_asset_authorization_private_ck CHECK (upload_purpose NOT IN ('PRODUCT_IMAGE','BRAND_IMAGE','PASTRY_REFERENCE') OR visibility='PRIVATE');
