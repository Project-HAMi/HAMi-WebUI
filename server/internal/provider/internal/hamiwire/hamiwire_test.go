package hamiwire

import "testing"

func TestDecodeRegistrations7And9FieldsAndJSON(t *testing.T) {
	recs7, err := DecodeRegistrations("dev-0,10,16384,100,ModelX,1,true:")
	if err != nil || len(recs7) != 1 {
		t.Fatalf("7-field = (%+v, %v), want 1 record", recs7, err)
	}
	if recs7[0].ID != "dev-0" || recs7[0].Numa != 1 || !recs7[0].Health || recs7[0].Known["mode"] {
		t.Fatalf("unexpected 7-field record: %+v", recs7[0])
	}

	recs9, err := DecodeRegistrations("dev-1,4,81920,100,ModelY,0,false,2,mig:")
	if err != nil || len(recs9) != 1 {
		t.Fatalf("9-field = (%+v, %v), want 1 record", recs9, err)
	}
	if recs9[0].Index != 2 || recs9[0].Mode != "mig" || !recs9[0].Known["mode"] || recs9[0].Health || !recs9[0].Known["health"] {
		t.Fatalf("unexpected 9-field record: %+v", recs9[0])
	}

	// JSON with omitempty-absent fields still keeps the record without fabricating unknown scalars.
	jsonRecs, err := DecodeRegistrations(`[{"id":"dev-json","type":"ModelZ","count":4,"devmem":0}]`)
	if err != nil || len(jsonRecs) != 1 {
		t.Fatalf("JSON = (%+v, %v), want 1 record", jsonRecs, err)
	}
	r := jsonRecs[0]
	if r.Known["devcore"] || r.Known["health"] || !r.Known["count"] || !r.Known["devmem"] || r.Devmem != 0 {
		t.Fatalf("unexpected JSON field presence: %+v", r)
	}
}

func TestDecodeRegistrationsRejectsDuplicatesAndInvalidFraming(t *testing.T) {
	if _, err := DecodeRegistrations("no-colon"); err == nil {
		t.Fatal("expected error for missing colon separator")
	}
	recs, err := DecodeRegistrations("dup,1,10,10,A,0,true:dup,1,10,10,A,0,true:ok,2,20,50,B,0,true:")
	if err == nil || len(recs) != 1 || recs[0].ID != "ok" {
		t.Fatalf("duplicate filter = (%+v, %v), want only 'ok' + error", recs, err)
	}
}

func TestDecodeContainerAllocations(t *testing.T) {
	segs, err := DecodeContainerAllocations("d0,TypeA,4096,0,extra:d1,TypeA,2048,50:")
	if err != nil || len(segs) != 2 {
		t.Fatalf("DecodeContainerAllocations = (%+v, %v)", segs, err)
	}
	if segs[0].ID != "d0" || segs[0].Mem != 4096 || segs[0].Cores != 0 || segs[1].Cores != 50 {
		t.Fatalf("unexpected segments: %+v", segs)
	}
	if _, err := ParseAllocationSegment("d0,TypeA,-1,50"); err == nil {
		t.Fatal("expected negative memory error")
	}
}
