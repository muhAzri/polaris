// Package e2e drives the whole API over HTTP against a real Postgres to
// check the behaviours that should match Moodle: per-course roles, context
// inheritance, permission overrides, enrolment, hidden content, the
// gradebook, groups and the calendar.
//
// Run it against a throwaway database (its schema is dropped):
//
//	docker compose exec -T postgres psql -U polaris postgres -c "create database polaris_e2e"
//	E2E_DATABASE_URL='postgres://polaris:polaris@localhost:5433/polaris_e2e?sslmode=disable' go test ./e2e/
package e2e

import (
	"strings"
	"testing"
)

func TestParity(t *testing.T) {
	e := newEnv(t)

	budi := e.newUser("Budi Santoso", "budi.santoso@gmail.com", "teacher")
	hendra := e.newUser("Hendra Gunawan", "hendra.gunawan@gmail.com", "teacher")
	rina := e.newUser("Rina Wijaya", "rina.wijaya@gmail.com", "")
	dewi := e.newUser("Dewi Kusuma", "dewi.kusuma@gmail.com", "")
	siti := e.newUser("Siti Rahmawati", "siti.rahmawati@gmail.com", "")
	agus := e.newUser("Agus Pratama", "agus.pratama@gmail.com", "admin")

	var cid, cid2 string
	var sec1, sec2, sec3, page, label, urlm, book string

	t.Run("courses and per-course roles", func(t *testing.T) {
		check(t, "student cannot create course", e.status("POST", "/courses", rina.token, J{"title": "Kelas Rahasia"}), 403)
		c := e.must(201, "POST", "/courses", budi.token, J{"title": "Statistika Dasar", "short_name": "stat-101", "format": "weeks"})
		cid = str(c, "id")
		check(t, "short name kept", str(c, "short_name"), "stat-101")
		check(t, "format kept", str(c, "format"), "weeks")
		check(t, "duplicate short name is case-insensitive", e.status("POST", "/courses", budi.token, J{"title": "Duplikat", "short_name": "STAT-101"}), 409)
		cid2 = str(e.must(201, "POST", "/courses", hendra.token, J{"title": "Kalkulus Lanjut"}), "id")

		check(t, "teacher of another course cannot edit", e.status("PUT", "/courses/"+cid, hendra.token, J{"title": "Dibajak"}), 403)
		check(t, "owner edits course", e.status("PUT", "/courses/"+cid, budi.token, J{"title": "Statistika Dasar I", "description": "Pengantar statistik deskriptif"}), 200)
		check(t, "non-enrolled cannot open course", e.status("GET", "/courses/"+cid, siti.token, nil), 403)

		check(t, "manual enrol student", e.status("POST", "/courses/"+cid+"/enrolments", budi.token, J{"email": rina.email}), 204)
		check(t, "manual enrol second student", e.status("POST", "/courses/"+cid+"/enrolments", budi.token, J{"email": dewi.email, "role": "student"}), 204)
		p := e.must(200, "GET", "/courses/"+cid+"/participants", budi.token, nil)
		check(t, "participants count", len(list(p)), 3)
		check(t, "owner has teacher role", get(find(p, "name", "Budi Santoso"), "roles"), []any{"teacher"})
		check(t, "student role", get(find(p, "name", "Rina Wijaya"), "roles"), []any{"student"})
		check(t, "student cannot list participants", e.status("GET", "/courses/"+cid+"/participants", rina.token, nil), 403)
		check(t, "student cannot edit course", e.status("PUT", "/courses/"+cid, rina.token, J{"title": "x"}), 403)

		check(t, "student lists only their course", colStr(e.must(200, "GET", "/courses", rina.token, nil), "title"), []string{"Statistika Dasar I"})
		check(t, "admin lists all courses", len(list(e.must(200, "GET", "/courses", agus.token, nil))), 2)
		caps := e.must(200, "GET", "/courses/"+cid+"/my-capabilities", rina.token, nil)
		check(t, "student lacks course:update", contains(caps, "course:update"), false)
		caps = e.must(200, "GET", "/courses/"+cid+"/my-capabilities", budi.token, nil)
		check(t, "teacher has grade:manage", contains(caps, "grade:manage"), true)
	})

	t.Run("self enrolment", func(t *testing.T) {
		path := "/courses/" + cid
		check(t, "disabled by default", e.status("POST", path+"/enroll", siti.token, J{}), 403)
		check(t, "configure", e.status("PUT", path+"/self-enrolment", budi.token, J{"enabled": true, "key": "belajar2026", "max_users": 1, "role": "student"}), 204)
		av := e.must(200, "GET", "/courses/available", siti.token, nil)
		check(t, "available course requires key", []any{len(list(av)), get(list(av)[0], "requires_key")}, []any{1, true})
		check(t, "wrong key", e.status("POST", path+"/enroll", siti.token, J{"key": "salah"}), 403)
		check(t, "right key enrols", e.status("POST", path+"/enroll", siti.token, J{"key": "belajar2026"}), 204)
		check(t, "self-enrolled can open course", e.status("GET", path, siti.token, nil), 200)
		check(t, "suspend enrolment", e.status("PATCH", path+"/enrolments/"+siti.id, budi.token, J{"status": "suspended"}), 204)
		check(t, "suspended cannot open course", e.status("GET", path, siti.token, nil), 403)
		check(t, "reactivate", e.status("PATCH", path+"/enrolments/"+siti.id, budi.token, J{"status": "active"}), 204)
		check(t, "unenrol", e.status("DELETE", path+"/enrolments/"+siti.id, budi.token, nil), 204)
		check(t, "unenrolled cannot open course", e.status("GET", path, siti.token, nil), 403)
		check(t, "disable self enrolment", e.status("PUT", path+"/self-enrolment", budi.token, J{"enabled": false, "role": "student"}), 204)
	})

	t.Run("sections and modules", func(t *testing.T) {
		sec1 = str(e.must(201, "POST", "/courses/"+cid+"/sections", budi.token, J{"title": "Minggu 1", "summary": "Statistik deskriptif"}), "id")
		sec2 = str(e.must(201, "POST", "/courses/"+cid+"/sections", budi.token, J{"title": "Minggu 2"}), "id")
		sec3 = str(e.must(201, "POST", "/courses/"+cid+"/sections", budi.token, J{"title": "Minggu 3"}), "id")
		moved := e.must(200, "PATCH", "/sections/"+sec3, budi.token, J{"position": 1})
		check(t, "reorder section", get(moved, "position"), 1)
		secs := e.must(200, "GET", "/courses/"+cid+"/sections", budi.token, nil)
		check(t, "section order", colStr(secs, "title"), []string{"General", "Minggu 3", "Minggu 1", "Minggu 2"})
		general := str(list(secs)[0], "id")
		check(t, "general section cannot move", e.status("PATCH", "/sections/"+general, budi.token, J{"position": 2}), 409)
		check(t, "general section cannot be deleted", e.status("DELETE", "/sections/"+general, budi.token, nil), 409)
		check(t, "hide section", e.status("PATCH", "/sections/"+sec3, budi.token, J{"visible": false}), 200)

		page = str(e.must(201, "POST", "/sections/"+sec1+"/modules/page", budi.token, J{"title": "Ukuran Pemusatan", "content": "Mean, median, modus"}), "module", "id")
		label = str(e.must(201, "POST", "/sections/"+sec1+"/modules/label", budi.token, J{"content": "Baca dulu bab 1"}), "module", "id")
		urlm = str(e.must(201, "POST", "/sections/"+sec1+"/modules/url", budi.token, J{"title": "Kaggle", "url": "https://kaggle.com"}), "module", "id")
		book = str(e.must(201, "POST", "/sections/"+sec1+"/modules/book", budi.token, J{"title": "Modul Statistik"}), "module", "id")
		check(t, "student cannot create module", e.status("POST", "/sections/"+sec1+"/modules/label", rina.token, J{"content": "x"}), 403)
		check(t, "teacher of another course cannot create module", e.status("POST", "/sections/"+sec1+"/modules/label", hendra.token, J{"content": "x"}), 403)

		e.must(201, "POST", "/modules/"+book+"/book-chapters", budi.token, J{"title": "Bab 1", "content": "isi"})
		ch2 := e.must(201, "POST", "/modules/"+book+"/book-chapters", budi.token, J{"title": "1.1 Mean", "content": "isi", "subchapter": true})
		check(t, "subchapter", get(ch2, "subchapter"), true)
		ch3 := e.must(201, "POST", "/modules/"+book+"/book-chapters", budi.token, J{"title": "Bab 2", "content": "isi"})
		r := e.must(200, "PATCH", "/modules/"+book+"/book-chapters/"+str(ch3, "id"), budi.token, J{"position": 0})
		check(t, "reorder chapter to first", []any{get(r, "position"), get(r, "subchapter")}, []any{0, false})
		check(t, "hide chapter", e.status("PATCH", "/modules/"+book+"/book-chapters/"+str(ch2, "id"), budi.token, J{"hidden": true}), 200)
		check(t, "edit page content", e.status("PUT", "/modules/"+page+"/content", budi.token, J{"title": "Ukuran Pemusatan Data", "content": "Mean, median, modus, kuartil"}), 204)
		check(t, "hide page", e.status("PATCH", "/modules/"+page, budi.token, J{"visible": false}), 200)

		cont := e.must(200, "GET", "/courses/"+cid+"/content", rina.token, nil)
		check(t, "student sees no hidden section or page", sectionTypes(cont), []string{"General:", "Minggu 1:label,url,book", "Minggu 2:"})
		check(t, "student book omits hidden chapter", colStr(get(moduleOfType(cont, "book"), "data", "chapters"), "title"), []string{"Bab 2", "Bab 1"})
		cont = e.must(200, "GET", "/courses/"+cid+"/content", budi.token, nil)
		check(t, "teacher sees hidden section", colStr(cont, "title"), []string{"General", "Minggu 3", "Minggu 1", "Minggu 2"})
		check(t, "teacher sees hidden page", moduleOfType(cont, "page") != nil, true)
		check(t, "teacher book shows all chapters", len(list(get(moduleOfType(cont, "book"), "data", "chapters"))), 3)

		check(t, "set availability window", e.status("PATCH", "/modules/"+page, budi.token, J{"visible": true, "available_from": "2099-01-01T00:00:00Z"}), 200)
		pg := moduleOfType(e.must(200, "GET", "/courses/"+cid+"/content", rina.token, nil), "page")
		check(t, "restricted stub has no data", []any{get(pg, "restricted"), get(pg, "data")}, []any{true, nil})
		mv := e.must(200, "PATCH", "/modules/"+page, budi.token, J{"clear_availability": true, "section_id": sec2, "position": 0})
		check(t, "moved to another section", get(mv, "section_id"), sec2)
		cont = e.must(200, "GET", "/courses/"+cid+"/content", rina.token, nil)
		check(t, "page now in Minggu 2", sectionTypes(cont)[1:], []string{"Minggu 1:label,url,book", "Minggu 2:page"})

		check(t, "delete module", e.status("DELETE", "/modules/"+label, budi.token, nil), 204)
		cont = e.must(200, "GET", "/courses/"+cid+"/content", rina.token, nil)
		var positions []any
		for _, s := range list(cont) {
			if str(s, "title") == "Minggu 1" {
				positions = col(get(s, "modules"), "position")
			}
		}
		check(t, "positions closed after delete", positions, []any{0.0, 1.0})
		check(t, "invalid group mode", e.status("PATCH", "/modules/"+urlm, budi.token, J{"group_mode": "bogus"}), 400)
		check(t, "other teacher cannot edit module", e.status("PATCH", "/modules/"+urlm, hendra.token, J{"visible": false}), 403)
		check(t, "delete section with modules", e.status("DELETE", "/sections/"+sec2, budi.token, nil), 204)
		check(t, "positions renumbered", col(e.must(200, "GET", "/courses/"+cid+"/sections", budi.token, nil), "position"), []any{0.0, 1.0, 2.0})
	})

	t.Run("roles, overrides and category inheritance", func(t *testing.T) {
		path := "/courses/" + cid
		check(t, "teacher assigns non-editing teacher", e.status("POST", path+"/role-assignments", budi.token, J{"user_id": dewi.id, "role": "nonediting_teacher"}), 204)
		caps := e.must(200, "GET", path+"/my-capabilities", dewi.token, nil)
		check(t, "non-editing teacher sees all grades", contains(caps, "grade:viewall"), true)
		check(t, "cannot assign admin at course level", e.status("POST", path+"/role-assignments", budi.token, J{"user_id": dewi.id, "role": "admin"}), 403)
		check(t, "remove role", e.status("DELETE", path+"/role-assignments/"+dewi.id+"/nonediting_teacher", budi.token, nil), 204)
		check(t, "teacher cannot override permissions", e.status("PUT", path+"/role-overrides", budi.token, J{"role": "student", "capability": "grade:view", "permission": "prohibit"}), 403)

		check(t, "student cannot create category", e.status("POST", "/categories", rina.token, J{"name": "Rahasia"}), 403)
		catid := str(e.must(201, "POST", "/categories", agus.token, J{"name": "Sains & Matematika"}), "id")
		subid := str(e.must(201, "POST", "/categories", agus.token, J{"name": "Statistika", "parent_id": catid}), "id")
		check(t, "category cycle rejected", e.status("PUT", "/categories/"+catid, agus.token, J{"name": "Sains", "parent_id": subid}), 400)
		check(t, "admin assigns manager at category", e.status("POST", "/categories/"+subid+"/role-assignments", agus.token, J{"user_id": siti.id, "role": "manager"}), 204)
		check(t, "manager does not reach course outside category", e.status("GET", path, siti.token, nil), 403)
		check(t, "admin moves course into category", e.status("PUT", path, agus.token, J{"title": "Statistika Dasar I", "category_id": subid}), 200)
		check(t, "manager reaches course via inheritance", e.status("GET", path, siti.token, nil), 200)
		check(t, "category manager edits course", e.status("PUT", path, siti.token, J{"title": "Statistika Dasar I", "description": "diubah manajer"}), 200)
		check(t, "category manager creates course in category", e.status("POST", "/courses", siti.token, J{"title": "Kursus Baru", "category_id": subid}), 201)
		check(t, "but not at site level", e.status("POST", "/courses", siti.token, J{"title": "Kursus Luar"}), 403)

		check(t, "manager overrides student grade:view", e.status("PUT", path+"/role-overrides", siti.token, J{"role": "student", "capability": "grade:view", "permission": "prohibit"}), 204)
		check(t, "prohibit denies student", e.status("GET", path+"/gradebook/mine", rina.token, nil), 403)
		check(t, "clear override", e.status("PUT", path+"/role-overrides", siti.token, J{"role": "student", "capability": "grade:view", "permission": "notset"}), 204)
		check(t, "student allowed again", e.status("GET", path+"/gradebook/mine", rina.token, nil), 200)
		check(t, "cannot delete non-empty category", e.status("DELETE", "/categories/"+subid, agus.token, nil), 409)
		check(t, "delete with move_to", e.status("DELETE", "/categories/"+subid+"?move_to="+catid, agus.token, nil), 204)

		student := find(e.must(200, "GET", "/roles", agus.token, nil), "name", "student")
		role := e.must(201, "POST", "/admin/roles", agus.token, J{"name": "asisten_lab", "display_name": "Asisten Lab", "clone_from": student["id"]})
		roleID := str(role, "id")
		check(t, "set capability on custom role", e.status("PUT", "/admin/roles/"+roleID+"/capabilities/grade:edit", agus.token, J{"permission": "allow"}), 204)
		d := e.must(200, "GET", "/admin/roles/"+roleID, agus.token, nil)
		check(t, "custom role has cloned and new capabilities", []any{get(d, "permissions", "mod:view"), get(d, "permissions", "grade:edit")}, []any{"allow", "allow"})
		check(t, "delete custom role", e.status("DELETE", "/admin/roles/"+roleID, agus.token, nil), 204)
		check(t, "teacher cannot use role admin", e.status("GET", "/admin/capabilities", budi.token, nil), 403)
	})

	t.Run("gradebook", func(t *testing.T) {
		path := "/courses/" + cid
		cats := e.must(200, "GET", path+"/gradebook/categories", budi.token, nil)
		check(t, "root category exists", len(list(cats)), 1)
		root := str(find(cats, "is_root", true), "id")
		quiz := e.must(201, "POST", path+"/gradebook/categories", budi.token, J{"name": "Kuis", "aggregation": "mean", "drop_lowest": 1})
		quizID := str(quiz, "id")
		check(t, "nested category", get(quiz, "parent_id"), root)
		check(t, "invalid aggregation", e.status("POST", path+"/gradebook/categories", budi.token, J{"name": "Salah", "aggregation": "bogus"}), 400)

		item := func(name, cat string, max float64, extra J) string {
			body := J{"name": name, "max_grade": max}
			if cat != "" {
				body["category_id"] = cat
			}
			for k, v := range extra {
				body[k] = v
			}
			return str(e.must(201, "POST", path+"/gradebook/items", budi.token, body), "id")
		}
		k1, k2, k3 := item("Kuis 1", quizID, 100, nil), item("Kuis 2", quizID, 100, nil), item("Kuis 3", quizID, 100, nil)
		uts := item("UTS", "", 50, J{"weight": 3})
		bonus := item("Bonus rahasia", "", 100, J{"hidden": true})
		grade := func(itemID string, who person, tok string, body J) int {
			return e.status("POST", "/gradebook/items/"+itemID+"/grades/"+who.id, tok, body)
		}

		check(t, "student cannot enter grades", grade(k1, rina, rina.token, J{"grade": 90}), 403)
		check(t, "grade over max rejected", grade(k1, rina, budi.token, J{"grade": 101}), 400)
		check(t, "grade below min rejected", grade(k1, rina, budi.token, J{"grade": -1}), 400)
		for id, g := range map[string]float64{k1: 60, k2: 90, k3: 80, uts: 40, bonus: 100} {
			check(t, "enter grade", grade(id, rina, budi.token, J{"grade": g}), 200)
		}

		rep := e.must(200, "GET", path+"/gradebook/mine", rina.token, nil)
		check(t, "quiz total drops lowest", get(rep, "categories", quizID, "grade"), 85.0)
		check(t, "course total leaves out hidden item", get(rep, "course_total", "grade"), 82.5)
		check(t, "letter grade", get(rep, "course_total", "letter"), "B-")
		check(t, "student does not see hidden item", itemNamed(rep, "Bonus rahasia"), false)

		gr := e.must(200, "GET", path+"/gradebook/report", budi.token, nil)
		st := find(get(gr, "students"), "name", "Rina Wijaya")
		check(t, "grader total includes hidden item", get(st, "course_total", "grade") != 82.5, true)
		check(t, "grader report lists only students", colStr(get(gr, "students"), "name"), []string{"Dewi Kusuma", "Rina Wijaya"})
		check(t, "student cannot open grader report", e.status("GET", path+"/gradebook/report", rina.token, nil), 403)

		check(t, "lock grade", grade(k1, rina, budi.token, J{"locked": true}), 200)
		check(t, "locked grade refuses edit", grade(k1, rina, budi.token, J{"grade": 70}), 409)
		check(t, "unlock and edit together", grade(k1, rina, budi.token, J{"grade": 70, "locked": false}), 200)
		h := e.must(200, "GET", "/gradebook/items/"+k1+"/grades/"+rina.id+"/history", budi.token, nil)
		check(t, "history recorded", []any{get(list(h)[0], "old_grade"), get(list(h)[0], "new_grade"), get(list(h)[1], "old_grade"), get(list(h)[1], "new_grade")}, []any{60.0, 70.0, nil, 60.0})
		check(t, "exclude a grade", grade(uts, rina, budi.token, J{"excluded": true}), 200)
		rep = e.must(200, "GET", path+"/gradebook/mine", rina.token, nil)
		check(t, "excluded grade leaves quiz only", get(rep, "course_total", "grade"), get(rep, "categories", quizID, "grade"))
		check(t, "change aggregation", e.status("PATCH", "/gradebook/categories/"+quizID, budi.token, J{"aggregation": "natural", "drop_lowest": 0}), 200)
		rep = e.must(200, "GET", path+"/gradebook/mine", rina.token, nil)
		check(t, "natural sums points", []any{get(rep, "categories", quizID, "grade"), get(rep, "categories", quizID, "max")}, []any{240.0, 300.0})
		check(t, "root cannot be moved", e.status("PATCH", "/gradebook/categories/"+root, budi.token, J{"parent_id": quizID}), 409)
		check(t, "root cannot be deleted", e.status("DELETE", "/gradebook/categories/"+root, budi.token, nil), 409)
		check(t, "delete category keeps items", e.status("DELETE", "/gradebook/categories/"+quizID, budi.token, nil), 204)
		items := e.must(200, "GET", path+"/gradebook/items", budi.token, nil)
		allRoot := true
		for _, it := range list(items) {
			allRoot = allRoot && str(it, "category_id") == root
		}
		check(t, "items moved to parent", allRoot, true)
		check(t, "student item list omits hidden", contains(colAny(e.must(200, "GET", path+"/gradebook/items", rina.token, nil), "name"), "Bonus rahasia"), false)
		check(t, "delete manual item", e.status("DELETE", "/gradebook/items/"+bonus, budi.token, nil), 204)
		check(t, "student cannot list all grades", e.status("GET", "/gradebook/items/"+k1+"/grades", rina.token, nil), 403)
		check(t, "other student cannot either", e.status("GET", "/gradebook/items/"+k1+"/grades", dewi.token, nil), 403)
	})

	t.Run("groups", func(t *testing.T) {
		path := "/courses/" + cid
		g1 := e.must(201, "POST", path+"/groups", budi.token, J{"name": "Kelompok A"})
		gid := str(g1, "id")
		check(t, "duplicate group name", e.status("POST", path+"/groups", budi.token, J{"name": "Kelompok A"}), 409)
		check(t, "add enrolled member", e.status("POST", "/groups/"+gid+"/members", budi.token, J{"user_id": rina.id}), 204)
		check(t, "cannot add non-enrolled user", e.status("POST", "/groups/"+gid+"/members", budi.token, J{"user_id": hendra.id}), 400)
		check(t, "rename group", e.status("PATCH", "/groups/"+gid, budi.token, J{"name": "Tim Alfa"}), 200)
		check(t, "remove member", e.status("DELETE", "/groups/"+gid+"/members/"+rina.id, budi.token, nil), 204)
		check(t, "student cannot create group", e.status("POST", path+"/groups", rina.token, J{"name": "Curang"}), 403)

		auto := e.must(201, "POST", path+"/groups/auto", budi.token, J{"count": 2, "prefix": "Tim"})
		members := 0
		for _, g := range list(auto) {
			if strings.HasPrefix(str(g, "name"), "Tim ") && !strings.HasPrefix(str(g, "name"), "Tim Alfa") {
				members += len(list(get(g, "members")))
			}
		}
		check(t, "auto-create places every student", members, 2)

		gi := e.must(201, "POST", path+"/groupings", budi.token, J{"name": "Praktikum"})
		giID := str(gi, "id")
		check(t, "add group to grouping", e.status("POST", "/groupings/"+giID+"/groups", budi.token, J{"group_id": gid}), 204)
		gl := e.must(200, "GET", path+"/groupings", rina.token, nil)
		check(t, "grouping lists its group", get(list(gl)[0], "group_ids"), []any{gid})
		r := e.must(200, "PATCH", "/modules/"+urlm, budi.token, J{"group_mode": "separate", "grouping_id": giID})
		check(t, "module group mode", get(r, "group_mode"), "separate")
		check(t, "foreign grouping rejected", e.status("PATCH", "/modules/"+urlm, budi.token, J{"grouping_id": "00000000-0000-4000-8000-000000000000"}), 400)
	})

	t.Run("calendar", func(t *testing.T) {
		path := "/courses/" + cid
		ev := e.must(201, "POST", path+"/events", budi.token, J{"name": "Kuliah tamu", "start_at": "2026-10-05T09:00:00Z", "end_at": "2026-10-05T11:00:00Z", "repeat_rule": "weekly", "repeat_until": "2026-10-26T23:59:59Z"})
		evID := str(ev, "id")
		check(t, "student cannot create course event", e.status("POST", path+"/events", rina.token, J{"name": "Palsu", "start_at": "2026-10-05T09:00:00Z"}), 403)
		check(t, "bad repeat rule", e.status("POST", path+"/events", budi.token, J{"name": "x", "start_at": "2026-10-05T09:00:00Z", "repeat_rule": "yearly"}), 400)

		month := "/users/me/events?from=2026-10-01T00:00:00Z&to=2026-10-31T23:59:59Z"
		check(t, "four weekly occurrences", len(list(e.must(200, "GET", month, rina.token, nil))), 4)
		check(t, "range narrows occurrences", len(list(e.must(200, "GET", "/users/me/events?from=2026-10-12T00:00:00Z&to=2026-10-19T23:59:59Z", rina.token, nil))), 2)
		check(t, "student cannot edit course event", e.status("PATCH", "/events/"+evID, rina.token, J{"name": "Diubah"}), 403)
		r := e.must(200, "PATCH", "/events/"+evID, budi.token, J{"name": "Kuliah tamu (ruang B)"})
		check(t, "teacher edits course event", get(r, "name"), "Kuliah tamu (ruang B)")

		mine := str(e.must(201, "POST", "/users/me/events", rina.token, J{"name": "Belajar kelompok", "start_at": "2026-10-07T13:00:00Z"}), "id")
		check(t, "others cannot edit personal event", e.status("PATCH", "/events/"+mine, dewi.token, J{"name": "x"}), 403)
		check(t, "owner deletes personal event", e.status("DELETE", "/events/"+mine, rina.token, nil), 204)
		check(t, "teacher cannot create site event", e.status("POST", "/admin/events", budi.token, J{"name": "Libur", "start_at": "2026-10-17T00:00:00Z"}), 403)
		check(t, "admin creates site event", e.status("POST", "/admin/events", agus.token, J{"name": "Libur nasional", "start_at": "2026-10-17T00:00:00Z"}), 201)

		_, ics := e.do("GET", "/users/me/calendar.ics", rina.token, nil)
		body, _ := ics.(string)
		check(t, "ics export", []any{strings.Contains(body, "BEGIN:VCALENDAR"), strings.Contains(body, "RRULE:FREQ=WEEKLY")}, []any{true, true})

		e.sql(`INSERT INTO calendar_events (event_type, course_id, course_module_id, event_kind, name, start_at)
			VALUES ('course', $1, $2, 'due', 'Batas kumpul', '2026-10-20T00:00:00Z')`, cid, urlm)
		day := "/users/me/events?from=2026-10-20T00:00:00Z&to=2026-10-21T00:00:00Z"
		check(t, "module event visible while module visible", len(list(e.must(200, "GET", day, rina.token, nil))), 1)
		check(t, "hide module", e.status("PATCH", "/modules/"+urlm, budi.token, J{"visible": false}), 200)
		check(t, "hidden module's deadline hidden from student", len(list(e.must(200, "GET", day, rina.token, nil))), 0)
		teacherView := e.must(200, "GET", day, budi.token, nil)
		check(t, "teacher still sees it", len(list(teacherView)), 1)
		check(t, "module-owned event cannot be deleted directly", e.status("DELETE", "/events/"+str(list(teacherView)[0], "id"), budi.token, nil), 409)
	})

	t.Run("resource files", func(t *testing.T) {
		s, v := e.upload("/sections/"+sec1+"/modules/resource", budi.token, "Slide minggu 1", "slide.txt", "isi slide")
		check(t, "upload resource", s, 201)
		if len(e.store.files) != 1 {
			t.Errorf("expected one stored file, have %d", len(e.store.files))
		}
		check(t, "delete resource module", e.status("DELETE", "/modules/"+str(v, "module", "id"), budi.token, nil), 204)
		check(t, "stored file removed with the module", len(e.store.files), 0)
	})

	t.Run("deleting a course", func(t *testing.T) {
		check(t, "course teacher cannot delete", e.status("DELETE", "/courses/"+cid2, hendra.token, nil), 403)
		check(t, "admin deletes course", e.status("DELETE", "/courses/"+cid2, agus.token, nil), 204)
	})
}

func contains(v any, want string) bool {
	for _, x := range list(v) {
		if s, _ := x.(string); s == want {
			return true
		}
	}
	return false
}

func colAny(v any, key string) []any { return col(v, key) }

// itemNamed reports whether a user report lists a grade item by name.
func itemNamed(report any, name string) bool {
	for _, row := range list(get(report, "items")) {
		if str(row, "item", "name") == name {
			return true
		}
	}
	return false
}

// sectionTypes summarises course content as "Title:type,type" per section.
func sectionTypes(content any) []string {
	out := []string{}
	for _, s := range list(content) {
		types := []string{}
		for _, m := range list(get(s, "modules")) {
			types = append(types, str(m, "module_type"))
		}
		out = append(out, str(s, "title")+":"+strings.Join(types, ","))
	}
	return out
}

// moduleOfType returns the first module of a type anywhere in the content.
func moduleOfType(content any, moduleType string) J {
	for _, s := range list(content) {
		for _, m := range list(get(s, "modules")) {
			if str(m, "module_type") == moduleType {
				mm, _ := m.(J)
				return mm
			}
		}
	}
	return nil
}
