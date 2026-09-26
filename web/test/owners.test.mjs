import {test} from "node:test";
import assert from "node:assert/strict";
import {OwnersMap, UserCache} from "../root/js/owners.js";

test("owner ids are decoded from 24-bit RGB", () => {
    const m = new OwnersMap(2, 1);
    m.setFromRGBA(new Uint8Array([0x01, 0x23, 0x45, 255, 0, 0, 0, 255]));
    assert.equal(m.get(0, 0), 0x012345);
    assert.equal(m.get(1, 0), 0);
    assert.equal(m.get(5, 5), 0);
    m.set(1, 0, 42);
    assert.equal(m.get(1, 0), 42);
    m.set(-1, 0, 7); // ignored
    assert.ok(m.loaded);
});

test("user cache fetches once per id", async () => {
    let calls = 0;
    const cache = new UserCache(async ids => { calls++; return {users: {[ids[0]]: {id: ids[0], pseudo: "Brindille"}}}; });
    const [a, b] = await Promise.all([cache.get(1), cache.get(1)]);
    assert.equal(a.pseudo, "Brindille");
    assert.equal(b, a);
    assert.equal(calls, 1);
    assert.equal(cache.peek(1).pseudo, "Brindille");
    assert.equal(await cache.get(0), null);
});
