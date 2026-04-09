CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shop_id UUID NOT NULL REFERENCES shops(id),
    crawl_id UUID NOT NULL REFERENCES crawls(id),
    name TEXT NOT NULL,
    price DECIMAL(12, 2) NOT NULL CHECK (price > 0),
    image_url TEXT NOT NULL DEFAULT '',
    source_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_products_shop_id ON products(shop_id);
CREATE INDEX idx_products_crawl_id ON products(crawl_id);
