package productionbatch

import "testing"

func TestPreparedOutputUsesActualAcceptedOutput(t *testing.T) {
	actual, err := normalizeFraction(96, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	factor, err := normalizeFraction(1, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	output, err := preparedOutputInput(OperatingBatchStock, actual, factor, "prepared_item")
	if err != nil {
		t.Fatal(err)
	}
	if output == nil || output.Quantity != 96 {
		t.Fatalf("prepared output = %+v, want actual good output 96", output)
	}
}

func TestOnDemandDoesNotPostPreparedStock(t *testing.T) {
	actual, _ := normalizeFraction(100, 1, false)
	factor, _ := normalizeFraction(1, 1, false)
	output, err := preparedOutputInput(OperatingOnDemand, actual, factor, "prepared_item")
	if err != nil {
		t.Fatal(err)
	}
	if output != nil {
		t.Fatalf("ON_DEMAND produced inventory output: %+v", output)
	}
}

func TestBatchStockRequiresPreparedInventoryMapping(t *testing.T) {
	actual, _ := normalizeFraction(10, 1, false)
	factor, _ := normalizeFraction(1, 1, false)
	if _, err := preparedOutputInput(OperatingBatchStock, actual, factor, ""); err != ErrInventoryRequired {
		t.Fatalf("missing prepared inventory mapping error = %v", err)
	}
}
