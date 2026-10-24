# User Management API — สรุป Requirements

> ที่มา: `TEST-ASSIGNMENT.md` ส่วน **User Management API** เท่านั้น
> เอกสารนี้สรุปเฉพาะสิ่งที่โจทย์เขียนไว้ ไม่มีการตีความหรือเสนอวิธีทำ

**Objective (จากโจทย์):** สร้าง RESTful API ด้วย Golang สำหรับจัดการ users โดยใช้ MongoDB เก็บข้อมูล, ใช้ JWT สำหรับ authentication และเขียนโค้ดตามแนวทาง clean code

---

## 1. Requirements ที่ต้องทำ

### 1.1 User Model

user entity ต้องมี field ดังนี้

| Field | รายละเอียดตามโจทย์ |
| --- | --- |
| `ID` | auto-generated |
| `Name` | string |
| `Email` | string, unique |
| `Password` | hashed |
| `CreatedAt` | timestamp |

### 1.2 Authentication

ต้องมี

- User registration
- User authentication ที่คืนค่าเป็น JWT token

JWT requirements

- Protect endpoints ด้วย JWT
- Validate token ผ่าน middleware
- Sign token ด้วย HMAC (`HS256`) และ secret key

### 1.3 User Operations

- Create a new user
- Fetch a user by ID
- List all users
- Update a user's name or email
- Delete a user

### 1.4 MongoDB Integration

- ใช้ official Go MongoDB driver
- Persist และ retrieve ข้อมูล user จาก MongoDB

### 1.5 Middleware

- Logging middleware ที่บันทึก HTTP method, path และ execution time

### 1.6 Concurrency Task

- รัน background goroutine ทุก 10 วินาที เพื่อ log จำนวน users ทั้งหมดใน database

### 1.7 Testing

- เขียน unit tests ด้วย standard package `testing` ของ Go
- Mock การทำงานกับ MongoDB "where appropriate"

---

## 2. Bonus (Optional)

| หัวข้อ | รายละเอียดตามโจทย์ |
| --- | --- |
| Containerization | เพิ่ม Docker และ `docker-compose` สำหรับ API และ MongoDB |
| Abstraction | ใช้ Go interfaces เพื่อ abstract การทำงานกับ MongoDB เพื่อให้ test ได้ง่ายขึ้น |
| Validation | ทำ input validation (ตัวอย่างในโจทย์: required fields, email format) |
| Graceful Shutdown | จัดการ system signals โดยใช้ `context.Context` |
| gRPC Support | กำหนด `.proto` สำหรับ `CreateUser` และ `GetUser`; implement gRPC server (optionally secure ด้วย token metadata) |
| Hexagonal Architecture | จัดโครงสร้างแบบ ports and adapters; แยก domain, application, infrastructure layers; แยก business logic ออกจาก frameworks และ drivers |

---

## 3. Deliverables

Git repository ที่มี

- `README.md` พร้อมขั้นตอน setup และการรัน
- Guide อธิบายวิธี generate และใช้งาน JWT token
- ตัวอย่าง API requests และ responses
- เอกสาร assumptions หรือ design decisions

---

## 4. Evaluation Criteria

- Code quality, structure และ readability
- Correctness และ completeness ของ REST API
- Security และการ implement JWT
- การใช้งานและการ abstract MongoDB อย่างเหมาะสม
- Test coverage และการ mock ที่มีประสิทธิภาพ
- Idiomatic Go usage
- Bonus implementations (gRPC, Docker, validation, architecture)

---

## 5. จุดที่โจทย์เขียนไม่ชัด / กำกวม

> list ไว้เท่านั้น ยังไม่มีคำตอบ

### User Model
- รูปแบบของ `ID` ไม่ได้ระบุ (auto-generated แบบใด)
- ไม่ได้ระบุ hashing algorithm ของ `Password`
- ความ unique ของ `Email` ไม่ได้ระบุเรื่อง case sensitivity
- รูปแบบ/timezone ของ `CreatedAt` ไม่ได้ระบุ

### Authentication / JWT
- "User registration" กับ "Create a new user" (ข้อ 3) เป็นสิ่งเดียวกันหรือแยกกัน ไม่ได้ระบุ
- "User authentication" ใช้ credential อะไร (เช่น email/password) ไม่ได้ระบุชัด
- ไม่ได้ระบุว่า endpoint ไหนต้อง protect ด้วย JWT และ endpoint ไหนไม่ต้อง
- ไม่ได้ระบุ claims ที่ต้องมีใน token, อายุ token (expiry) หรือเรื่อง refresh token
- ไม่ได้ระบุว่า secret key มาจากไหน / ตั้งค่าอย่างไร
- ไม่ได้ระบุเรื่อง authorization (เช่น user หนึ่งแก้ไข/ลบ user อื่นได้หรือไม่, มี role หรือไม่)

### User Operations / REST API
- ไม่ได้ระบุ endpoint paths, HTTP methods, request/response format หรือ error format / status codes
- "Update a user's name or email" — แก้ได้ทีละ field หรือพร้อมกันได้, เป็น partial update หรือไม่ และแก้ password ได้หรือไม่ ไม่ได้ระบุ
- "List all users" — มี pagination / filtering หรือไม่ ไม่ได้ระบุ
- ไม่ได้ระบุว่า response ควรรวมหรือไม่รวม field ใดบ้าง (เช่น `Password`)
- Delete เป็นแบบ hard delete หรือ soft delete ไม่ได้ระบุ

### MongoDB
- ไม่ได้ระบุ version ของ MongoDB, ชื่อ database/collection หรือ index ที่ต้องมี

### Logging Middleware
- ไม่ได้ระบุ log format หรือปลายทางของ log
- ระบุเฉพาะ method, path, execution time — ไม่ได้บอกว่าต้องมีข้อมูลอื่น (เช่น status code) หรือไม่

### Concurrency Task
- ไม่ได้ระบุ log format/ปลายทาง และพฤติกรรมเมื่อ query จำนวน users ล้มเหลว
- ไม่ได้ระบุว่าต้องเริ่มนับรอบแรกทันทีหรือหลังผ่าน 10 วินาที

### Testing
- ขอบเขตของ "unit tests" (ต้องครอบคลุม layer ใดบ้าง) และเป้าหมาย coverage ไม่ได้ระบุ
- "Mock MongoDB interactions where appropriate" — "where appropriate" ไม่ได้นิยาม
- ข้อ 7 ให้ mock MongoDB แต่ Bonus "Abstraction" (ใช้ interfaces เพื่อ testability) ถูกจัดเป็น optional — ขอบเขตระหว่างสองข้อนี้ไม่ชัด

### Bonus
- gRPC: ต้องรันคู่กับ REST หรือไม่, "token metadata" หมายถึง JWT ตัวเดียวกันหรือไม่ ไม่ได้ระบุ
- Graceful Shutdown: ไม่ได้ระบุว่าต้องจัดการ signal ใดบ้าง
- Validation: ระบุไว้แค่ตัวอย่าง (required fields, email format) — กฎอื่น (เช่น ความยาว password/name) ไม่ได้ระบุ
- Containerization: ไม่ได้ระบุว่า Docker ต้องรองรับการรัน tests หรือเฉพาะการรัน API

### Tech Stack
- ไม่ได้ระบุ Go version หรือ HTTP framework/router ที่ต้องใช้

### Deliverables
- repository นี้มีทั้งสองส่วน (User Management และ Lottery) — ไม่ได้ระบุว่า `README.md` และเอกสารต่าง ๆ ต้องอยู่ที่ระดับใด
- ไม่ได้ระบุรูปแบบของ "sample API requests and responses" (เช่น curl, Postman collection, ฯลฯ)
- ไม่ได้ระบุรูปแบบ/ตำแหน่งของเอกสาร assumptions และ design decisions

### Evaluation Criteria
- ไม่ได้ระบุน้ำหนักคะแนนของแต่ละเกณฑ์ หรือสัดส่วนคะแนนของ Bonus
