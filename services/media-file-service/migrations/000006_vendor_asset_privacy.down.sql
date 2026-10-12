ALTER TABLE media_upload_authorizations DROP CONSTRAINT IF EXISTS vendor_asset_authorization_private_ck;
ALTER TABLE media_objects DROP CONSTRAINT IF EXISTS vendor_asset_private_ck;
