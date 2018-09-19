#include <stdio.h>
#include <stdlib.h>
#include <stdbool.h>
#include <math.h>
#include <stdint.h>
#undef NDEBUG
#include <assert.h>
/* Crash and burn. */

#include <stdarg.h>

static const char *fut_progname;

static void panic(int eval, const char *fmt, ...)
{
	va_list ap;

	va_start(ap, fmt);
        fprintf(stderr, "%s: ", fut_progname);
	vfprintf(stderr, fmt, ap);
	va_end(ap);
        exit(eval);
}

/* For generating arbitrary-sized error messages.  It is the callers
   responsibility to free the buffer at some point. */
static char* msgprintf(const char *s, ...) {
  va_list vl;
  va_start(vl, s);
  size_t needed = 1 + vsnprintf(NULL, 0, s, vl);
  char *buffer = malloc(needed);
  va_start(vl, s); /* Must re-init. */
  vsnprintf(buffer, needed, s, vl);
  return buffer;
}

/* Some simple utilities for wall-clock timing.

   The function get_wall_time() returns the wall time in microseconds
   (with an unspecified offset).
*/

#ifdef _WIN32

#include <windows.h>

static int64_t get_wall_time(void) {
  LARGE_INTEGER time,freq;
  assert(QueryPerformanceFrequency(&freq));
  assert(QueryPerformanceCounter(&time));
  return ((double)time.QuadPart / freq.QuadPart) * 1000000;
}

#else
/* Assuming POSIX */

#include <time.h>
#include <sys/time.h>

static int64_t get_wall_time(void) {
  struct timeval time;
  assert(gettimeofday(&time,NULL) == 0);
  return time.tv_sec * 1000000 + time.tv_usec;
}

#endif

#ifdef _MSC_VER
#define inline __inline
#endif
#include <string.h>
#include <inttypes.h>
#include <ctype.h>
#include <errno.h>
#include <assert.h>
/* A very simple cross-platform implementation of locks.  Uses
   pthreads on Unix and some Windows thing there.  Futhark's
   host-level code is not multithreaded, but user code may be, so we
   need some mechanism for ensuring atomic access to API functions.
   This is that mechanism.  It is not exposed to user code at all, so
   we do not have to worry about name collisions. */

#ifdef _WIN32

typedef HANDLE lock_t;

static lock_t create_lock(lock_t *lock) {
  *lock = CreateMutex(NULL,  /* Default security attributes. */
                      FALSE, /* Initially unlocked. */
                      NULL); /* Unnamed. */
}

static void lock_lock(lock_t *lock) {
  assert(WaitForSingleObject(*lock, INFINITE) == WAIT_OBJECT_0);
}

static void lock_unlock(lock_t *lock) {
  assert(ReleaseMutex(*lock));
}

static void free_lock(lock_t *lock) {
  CloseHandle(*lock);
}

#else
/* Assuming POSIX */

#include <pthread.h>

typedef pthread_mutex_t lock_t;

static void create_lock(lock_t *lock) {
  int r = pthread_mutex_init(lock, NULL);
  assert(r == 0);
}

static void lock_lock(lock_t *lock) {
  int r = pthread_mutex_lock(lock);
  assert(r == 0);
}

static void lock_unlock(lock_t *lock) {
  int r = pthread_mutex_unlock(lock);
  assert(r == 0);
}

static void free_lock(lock_t *lock) {
  /* Nothing to do for pthreads. */
  lock = lock;
}

#endif

struct memblock {
    int *references;
    char *mem;
    int64_t size;
    const char *desc;
} ;
struct futhark_cpu_context_config {
    int debugging;
} ;
struct futhark_cpu_context_config *futhark_cpu_context_config_new()
{
    struct futhark_cpu_context_config *cfg =
                                  malloc(sizeof(struct futhark_cpu_context_config));
    
    if (cfg == NULL)
        return NULL;
    cfg->debugging = 0;
    return cfg;
}
void futhark_cpu_context_config_free(struct futhark_cpu_context_config *cfg)
{
    free(cfg);
}
void futhark_cpu_context_config_set_debugging(struct futhark_cpu_context_config *cfg,
                                          int detail)
{
    cfg->debugging = detail;
}
void futhark_cpu_context_config_set_logging(struct futhark_cpu_context_config *cfg,
                                        int detail)
{
    /* Does nothing for this backend. */
    cfg = cfg;
    detail = detail;
}
struct futhark_cpu_context {
    int detail_memory;
    int debugging;
    lock_t lock;
    char *error;
    int64_t peak_mem_usage_default;
    int64_t cur_mem_usage_default;
} ;
struct futhark_cpu_context *futhark_cpu_context_new(struct futhark_cpu_context_config *cfg)
{
    struct futhark_cpu_context *ctx = malloc(sizeof(struct futhark_cpu_context));
    
    if (ctx == NULL)
        return NULL;
    ctx->detail_memory = cfg->debugging;
    ctx->debugging = cfg->debugging;
    ctx->error = NULL;
    create_lock(&ctx->lock);
    ctx->peak_mem_usage_default = 0;
    ctx->cur_mem_usage_default = 0;
    return ctx;
}
void futhark_cpu_context_free(struct futhark_cpu_context *ctx)
{
    free_lock(&ctx->lock);
    free(ctx);
}
int futhark_cpu_context_sync(struct futhark_cpu_context *ctx)
{
    ctx = ctx;
    return 0;
}
char *futhark_cpu_context_get_error(struct futhark_cpu_context *ctx)
{
    char *error = ctx->error;
    
    ctx->error = NULL;
    return error;
}
static void memblock_unref(struct futhark_cpu_context *ctx, struct memblock *block,
                           const char *desc)
{
    if (block->references != NULL) {
        *block->references -= 1;
        if (ctx->detail_memory)
            fprintf(stderr,
                    "Unreferencing block %s (allocated as %s) in %s: %d references remaining.\n",
                    desc, block->desc, "default space", *block->references);
        if (*block->references == 0) {
            ctx->cur_mem_usage_default -= block->size;
            free(block->mem);
            free(block->references);
            if (ctx->detail_memory)
                fprintf(stderr,
                        "%lld bytes freed (now allocated: %lld bytes)\n",
                        (long long) block->size,
                        (long long) ctx->cur_mem_usage_default);
        }
        block->references = NULL;
    }
}
static void memblock_alloc(struct futhark_cpu_context *ctx, struct memblock *block,
                           int64_t size, const char *desc)
{
    if (size < 0)
        panic(1, "Negative allocation of %lld bytes attempted for %s in %s.\n",
              (long long) size, desc, "default space",
              ctx->cur_mem_usage_default);
    memblock_unref(ctx, block, desc);
    block->mem = (char *) malloc(size);
    block->references = (int *) malloc(sizeof(int));
    *block->references = 1;
    block->size = size;
    block->desc = desc;
    ctx->cur_mem_usage_default += size;
    if (ctx->detail_memory)
        fprintf(stderr,
                "Allocated %lld bytes for %s in %s (now allocated: %lld bytes)",
                (long long) size, desc, "default space",
                (long long) ctx->cur_mem_usage_default);
    if (ctx->cur_mem_usage_default > ctx->peak_mem_usage_default) {
        ctx->peak_mem_usage_default = ctx->cur_mem_usage_default;
        if (ctx->detail_memory)
            fprintf(stderr, " (new peak).\n");
    } else if (ctx->detail_memory)
        fprintf(stderr, ".\n");
}
static void memblock_set(struct futhark_cpu_context *ctx, struct memblock *lhs,
                         struct memblock *rhs, const char *lhs_desc)
{
    memblock_unref(ctx, lhs, lhs_desc);
    (*rhs->references)++;
    *lhs = *rhs;
}
void futhark_cpu_debugging_report(struct futhark_cpu_context *ctx)
{
    if (ctx->detail_memory) {
        fprintf(stderr, "Peak memory usage for default space: %lld bytes.\n",
                (long long) ctx->peak_mem_usage_default);
    }
    if (ctx->debugging) { }
}
static int futrts_sum(struct futhark_cpu_context *ctx, double *out_scalar_out_4971,
                      int64_t col_mem_sizze_4951, struct memblock col_mem_4952,
                      int32_t sizze_4841);
static int futrts_mean(struct futhark_cpu_context *ctx, double *out_scalar_out_4972,
                       int64_t col_mem_sizze_4951, struct memblock col_mem_4952,
                       int32_t sizze_4848);
static int futrts_variance(struct futhark_cpu_context *ctx,
                           double *out_scalar_out_4973,
                           int64_t values_mem_sizze_4951,
                           struct memblock values_mem_4952, int32_t sizze_4857);
static int futrts_skew(struct futhark_cpu_context *ctx, double *out_scalar_out_4974,
                       int64_t values_mem_sizze_4951,
                       struct memblock values_mem_4952, int32_t sizze_4875);
static int futrts_kurtosis(struct futhark_cpu_context *ctx,
                           double *out_scalar_out_4975,
                           int64_t values_mem_sizze_4951,
                           struct memblock values_mem_4952, int32_t sizze_4902);
static int futrts_stddev(struct futhark_cpu_context *ctx,
                         double *out_scalar_out_4976,
                         int64_t values_mem_sizze_4951,
                         struct memblock values_mem_4952, int32_t sizze_4927);
static inline int8_t add8(int8_t x, int8_t y)
{
    return x + y;
}
static inline int16_t add16(int16_t x, int16_t y)
{
    return x + y;
}
static inline int32_t add32(int32_t x, int32_t y)
{
    return x + y;
}
static inline int64_t add64(int64_t x, int64_t y)
{
    return x + y;
}
static inline int8_t sub8(int8_t x, int8_t y)
{
    return x - y;
}
static inline int16_t sub16(int16_t x, int16_t y)
{
    return x - y;
}
static inline int32_t sub32(int32_t x, int32_t y)
{
    return x - y;
}
static inline int64_t sub64(int64_t x, int64_t y)
{
    return x - y;
}
static inline int8_t mul8(int8_t x, int8_t y)
{
    return x * y;
}
static inline int16_t mul16(int16_t x, int16_t y)
{
    return x * y;
}
static inline int32_t mul32(int32_t x, int32_t y)
{
    return x * y;
}
static inline int64_t mul64(int64_t x, int64_t y)
{
    return x * y;
}
static inline uint8_t udiv8(uint8_t x, uint8_t y)
{
    return x / y;
}
static inline uint16_t udiv16(uint16_t x, uint16_t y)
{
    return x / y;
}
static inline uint32_t udiv32(uint32_t x, uint32_t y)
{
    return x / y;
}
static inline uint64_t udiv64(uint64_t x, uint64_t y)
{
    return x / y;
}
static inline uint8_t umod8(uint8_t x, uint8_t y)
{
    return x % y;
}
static inline uint16_t umod16(uint16_t x, uint16_t y)
{
    return x % y;
}
static inline uint32_t umod32(uint32_t x, uint32_t y)
{
    return x % y;
}
static inline uint64_t umod64(uint64_t x, uint64_t y)
{
    return x % y;
}
static inline int8_t sdiv8(int8_t x, int8_t y)
{
    int8_t q = x / y;
    int8_t r = x % y;
    
    return q - ((r != 0 && r < 0 != y < 0) ? 1 : 0);
}
static inline int16_t sdiv16(int16_t x, int16_t y)
{
    int16_t q = x / y;
    int16_t r = x % y;
    
    return q - ((r != 0 && r < 0 != y < 0) ? 1 : 0);
}
static inline int32_t sdiv32(int32_t x, int32_t y)
{
    int32_t q = x / y;
    int32_t r = x % y;
    
    return q - ((r != 0 && r < 0 != y < 0) ? 1 : 0);
}
static inline int64_t sdiv64(int64_t x, int64_t y)
{
    int64_t q = x / y;
    int64_t r = x % y;
    
    return q - ((r != 0 && r < 0 != y < 0) ? 1 : 0);
}
static inline int8_t smod8(int8_t x, int8_t y)
{
    int8_t r = x % y;
    
    return r + (r == 0 || (x > 0 && y > 0) || (x < 0 && y < 0) ? 0 : y);
}
static inline int16_t smod16(int16_t x, int16_t y)
{
    int16_t r = x % y;
    
    return r + (r == 0 || (x > 0 && y > 0) || (x < 0 && y < 0) ? 0 : y);
}
static inline int32_t smod32(int32_t x, int32_t y)
{
    int32_t r = x % y;
    
    return r + (r == 0 || (x > 0 && y > 0) || (x < 0 && y < 0) ? 0 : y);
}
static inline int64_t smod64(int64_t x, int64_t y)
{
    int64_t r = x % y;
    
    return r + (r == 0 || (x > 0 && y > 0) || (x < 0 && y < 0) ? 0 : y);
}
static inline int8_t squot8(int8_t x, int8_t y)
{
    return x / y;
}
static inline int16_t squot16(int16_t x, int16_t y)
{
    return x / y;
}
static inline int32_t squot32(int32_t x, int32_t y)
{
    return x / y;
}
static inline int64_t squot64(int64_t x, int64_t y)
{
    return x / y;
}
static inline int8_t srem8(int8_t x, int8_t y)
{
    return x % y;
}
static inline int16_t srem16(int16_t x, int16_t y)
{
    return x % y;
}
static inline int32_t srem32(int32_t x, int32_t y)
{
    return x % y;
}
static inline int64_t srem64(int64_t x, int64_t y)
{
    return x % y;
}
static inline int8_t smin8(int8_t x, int8_t y)
{
    return x < y ? x : y;
}
static inline int16_t smin16(int16_t x, int16_t y)
{
    return x < y ? x : y;
}
static inline int32_t smin32(int32_t x, int32_t y)
{
    return x < y ? x : y;
}
static inline int64_t smin64(int64_t x, int64_t y)
{
    return x < y ? x : y;
}
static inline uint8_t umin8(uint8_t x, uint8_t y)
{
    return x < y ? x : y;
}
static inline uint16_t umin16(uint16_t x, uint16_t y)
{
    return x < y ? x : y;
}
static inline uint32_t umin32(uint32_t x, uint32_t y)
{
    return x < y ? x : y;
}
static inline uint64_t umin64(uint64_t x, uint64_t y)
{
    return x < y ? x : y;
}
static inline int8_t smax8(int8_t x, int8_t y)
{
    return x < y ? y : x;
}
static inline int16_t smax16(int16_t x, int16_t y)
{
    return x < y ? y : x;
}
static inline int32_t smax32(int32_t x, int32_t y)
{
    return x < y ? y : x;
}
static inline int64_t smax64(int64_t x, int64_t y)
{
    return x < y ? y : x;
}
static inline uint8_t umax8(uint8_t x, uint8_t y)
{
    return x < y ? y : x;
}
static inline uint16_t umax16(uint16_t x, uint16_t y)
{
    return x < y ? y : x;
}
static inline uint32_t umax32(uint32_t x, uint32_t y)
{
    return x < y ? y : x;
}
static inline uint64_t umax64(uint64_t x, uint64_t y)
{
    return x < y ? y : x;
}
static inline uint8_t shl8(uint8_t x, uint8_t y)
{
    return x << y;
}
static inline uint16_t shl16(uint16_t x, uint16_t y)
{
    return x << y;
}
static inline uint32_t shl32(uint32_t x, uint32_t y)
{
    return x << y;
}
static inline uint64_t shl64(uint64_t x, uint64_t y)
{
    return x << y;
}
static inline uint8_t lshr8(uint8_t x, uint8_t y)
{
    return x >> y;
}
static inline uint16_t lshr16(uint16_t x, uint16_t y)
{
    return x >> y;
}
static inline uint32_t lshr32(uint32_t x, uint32_t y)
{
    return x >> y;
}
static inline uint64_t lshr64(uint64_t x, uint64_t y)
{
    return x >> y;
}
static inline int8_t ashr8(int8_t x, int8_t y)
{
    return x >> y;
}
static inline int16_t ashr16(int16_t x, int16_t y)
{
    return x >> y;
}
static inline int32_t ashr32(int32_t x, int32_t y)
{
    return x >> y;
}
static inline int64_t ashr64(int64_t x, int64_t y)
{
    return x >> y;
}
static inline uint8_t and8(uint8_t x, uint8_t y)
{
    return x & y;
}
static inline uint16_t and16(uint16_t x, uint16_t y)
{
    return x & y;
}
static inline uint32_t and32(uint32_t x, uint32_t y)
{
    return x & y;
}
static inline uint64_t and64(uint64_t x, uint64_t y)
{
    return x & y;
}
static inline uint8_t or8(uint8_t x, uint8_t y)
{
    return x | y;
}
static inline uint16_t or16(uint16_t x, uint16_t y)
{
    return x | y;
}
static inline uint32_t or32(uint32_t x, uint32_t y)
{
    return x | y;
}
static inline uint64_t or64(uint64_t x, uint64_t y)
{
    return x | y;
}
static inline uint8_t xor8(uint8_t x, uint8_t y)
{
    return x ^ y;
}
static inline uint16_t xor16(uint16_t x, uint16_t y)
{
    return x ^ y;
}
static inline uint32_t xor32(uint32_t x, uint32_t y)
{
    return x ^ y;
}
static inline uint64_t xor64(uint64_t x, uint64_t y)
{
    return x ^ y;
}
static inline char ult8(uint8_t x, uint8_t y)
{
    return x < y;
}
static inline char ult16(uint16_t x, uint16_t y)
{
    return x < y;
}
static inline char ult32(uint32_t x, uint32_t y)
{
    return x < y;
}
static inline char ult64(uint64_t x, uint64_t y)
{
    return x < y;
}
static inline char ule8(uint8_t x, uint8_t y)
{
    return x <= y;
}
static inline char ule16(uint16_t x, uint16_t y)
{
    return x <= y;
}
static inline char ule32(uint32_t x, uint32_t y)
{
    return x <= y;
}
static inline char ule64(uint64_t x, uint64_t y)
{
    return x <= y;
}
static inline char slt8(int8_t x, int8_t y)
{
    return x < y;
}
static inline char slt16(int16_t x, int16_t y)
{
    return x < y;
}
static inline char slt32(int32_t x, int32_t y)
{
    return x < y;
}
static inline char slt64(int64_t x, int64_t y)
{
    return x < y;
}
static inline char sle8(int8_t x, int8_t y)
{
    return x <= y;
}
static inline char sle16(int16_t x, int16_t y)
{
    return x <= y;
}
static inline char sle32(int32_t x, int32_t y)
{
    return x <= y;
}
static inline char sle64(int64_t x, int64_t y)
{
    return x <= y;
}
static inline int8_t pow8(int8_t x, int8_t y)
{
    int8_t res = 1, rem = y;
    
    while (rem != 0) {
        if (rem & 1)
            res *= x;
        rem >>= 1;
        x *= x;
    }
    return res;
}
static inline int16_t pow16(int16_t x, int16_t y)
{
    int16_t res = 1, rem = y;
    
    while (rem != 0) {
        if (rem & 1)
            res *= x;
        rem >>= 1;
        x *= x;
    }
    return res;
}
static inline int32_t pow32(int32_t x, int32_t y)
{
    int32_t res = 1, rem = y;
    
    while (rem != 0) {
        if (rem & 1)
            res *= x;
        rem >>= 1;
        x *= x;
    }
    return res;
}
static inline int64_t pow64(int64_t x, int64_t y)
{
    int64_t res = 1, rem = y;
    
    while (rem != 0) {
        if (rem & 1)
            res *= x;
        rem >>= 1;
        x *= x;
    }
    return res;
}
static inline int8_t sext_i8_i8(int8_t x)
{
    return x;
}
static inline int16_t sext_i8_i16(int8_t x)
{
    return x;
}
static inline int32_t sext_i8_i32(int8_t x)
{
    return x;
}
static inline int64_t sext_i8_i64(int8_t x)
{
    return x;
}
static inline int8_t sext_i16_i8(int16_t x)
{
    return x;
}
static inline int16_t sext_i16_i16(int16_t x)
{
    return x;
}
static inline int32_t sext_i16_i32(int16_t x)
{
    return x;
}
static inline int64_t sext_i16_i64(int16_t x)
{
    return x;
}
static inline int8_t sext_i32_i8(int32_t x)
{
    return x;
}
static inline int16_t sext_i32_i16(int32_t x)
{
    return x;
}
static inline int32_t sext_i32_i32(int32_t x)
{
    return x;
}
static inline int64_t sext_i32_i64(int32_t x)
{
    return x;
}
static inline int8_t sext_i64_i8(int64_t x)
{
    return x;
}
static inline int16_t sext_i64_i16(int64_t x)
{
    return x;
}
static inline int32_t sext_i64_i32(int64_t x)
{
    return x;
}
static inline int64_t sext_i64_i64(int64_t x)
{
    return x;
}
static inline uint8_t zext_i8_i8(uint8_t x)
{
    return x;
}
static inline uint16_t zext_i8_i16(uint8_t x)
{
    return x;
}
static inline uint32_t zext_i8_i32(uint8_t x)
{
    return x;
}
static inline uint64_t zext_i8_i64(uint8_t x)
{
    return x;
}
static inline uint8_t zext_i16_i8(uint16_t x)
{
    return x;
}
static inline uint16_t zext_i16_i16(uint16_t x)
{
    return x;
}
static inline uint32_t zext_i16_i32(uint16_t x)
{
    return x;
}
static inline uint64_t zext_i16_i64(uint16_t x)
{
    return x;
}
static inline uint8_t zext_i32_i8(uint32_t x)
{
    return x;
}
static inline uint16_t zext_i32_i16(uint32_t x)
{
    return x;
}
static inline uint32_t zext_i32_i32(uint32_t x)
{
    return x;
}
static inline uint64_t zext_i32_i64(uint32_t x)
{
    return x;
}
static inline uint8_t zext_i64_i8(uint64_t x)
{
    return x;
}
static inline uint16_t zext_i64_i16(uint64_t x)
{
    return x;
}
static inline uint32_t zext_i64_i32(uint64_t x)
{
    return x;
}
static inline uint64_t zext_i64_i64(uint64_t x)
{
    return x;
}
static inline float fdiv32(float x, float y)
{
    return x / y;
}
static inline float fadd32(float x, float y)
{
    return x + y;
}
static inline float fsub32(float x, float y)
{
    return x - y;
}
static inline float fmul32(float x, float y)
{
    return x * y;
}
static inline float fmin32(float x, float y)
{
    return x < y ? x : y;
}
static inline float fmax32(float x, float y)
{
    return x < y ? y : x;
}
static inline float fpow32(float x, float y)
{
    return pow(x, y);
}
static inline char cmplt32(float x, float y)
{
    return x < y;
}
static inline char cmple32(float x, float y)
{
    return x <= y;
}
static inline float sitofp_i8_f32(int8_t x)
{
    return x;
}
static inline float sitofp_i16_f32(int16_t x)
{
    return x;
}
static inline float sitofp_i32_f32(int32_t x)
{
    return x;
}
static inline float sitofp_i64_f32(int64_t x)
{
    return x;
}
static inline float uitofp_i8_f32(uint8_t x)
{
    return x;
}
static inline float uitofp_i16_f32(uint16_t x)
{
    return x;
}
static inline float uitofp_i32_f32(uint32_t x)
{
    return x;
}
static inline float uitofp_i64_f32(uint64_t x)
{
    return x;
}
static inline int8_t fptosi_f32_i8(float x)
{
    return x;
}
static inline int16_t fptosi_f32_i16(float x)
{
    return x;
}
static inline int32_t fptosi_f32_i32(float x)
{
    return x;
}
static inline int64_t fptosi_f32_i64(float x)
{
    return x;
}
static inline uint8_t fptoui_f32_i8(float x)
{
    return x;
}
static inline uint16_t fptoui_f32_i16(float x)
{
    return x;
}
static inline uint32_t fptoui_f32_i32(float x)
{
    return x;
}
static inline uint64_t fptoui_f32_i64(float x)
{
    return x;
}
static inline double fdiv64(double x, double y)
{
    return x / y;
}
static inline double fadd64(double x, double y)
{
    return x + y;
}
static inline double fsub64(double x, double y)
{
    return x - y;
}
static inline double fmul64(double x, double y)
{
    return x * y;
}
static inline double fmin64(double x, double y)
{
    return x < y ? x : y;
}
static inline double fmax64(double x, double y)
{
    return x < y ? y : x;
}
static inline double fpow64(double x, double y)
{
    return pow(x, y);
}
static inline char cmplt64(double x, double y)
{
    return x < y;
}
static inline char cmple64(double x, double y)
{
    return x <= y;
}
static inline double sitofp_i8_f64(int8_t x)
{
    return x;
}
static inline double sitofp_i16_f64(int16_t x)
{
    return x;
}
static inline double sitofp_i32_f64(int32_t x)
{
    return x;
}
static inline double sitofp_i64_f64(int64_t x)
{
    return x;
}
static inline double uitofp_i8_f64(uint8_t x)
{
    return x;
}
static inline double uitofp_i16_f64(uint16_t x)
{
    return x;
}
static inline double uitofp_i32_f64(uint32_t x)
{
    return x;
}
static inline double uitofp_i64_f64(uint64_t x)
{
    return x;
}
static inline int8_t fptosi_f64_i8(double x)
{
    return x;
}
static inline int16_t fptosi_f64_i16(double x)
{
    return x;
}
static inline int32_t fptosi_f64_i32(double x)
{
    return x;
}
static inline int64_t fptosi_f64_i64(double x)
{
    return x;
}
static inline uint8_t fptoui_f64_i8(double x)
{
    return x;
}
static inline uint16_t fptoui_f64_i16(double x)
{
    return x;
}
static inline uint32_t fptoui_f64_i32(double x)
{
    return x;
}
static inline uint64_t fptoui_f64_i64(double x)
{
    return x;
}
static inline float fpconv_f32_f32(float x)
{
    return x;
}
static inline double fpconv_f32_f64(float x)
{
    return x;
}
static inline float fpconv_f64_f32(double x)
{
    return x;
}
static inline double fpconv_f64_f64(double x)
{
    return x;
}
static inline float futrts_log32(float x)
{
    return log(x);
}
static inline float futrts_log2_32(float x)
{
    return log2(x);
}
static inline float futrts_log10_32(float x)
{
    return log10(x);
}
static inline float futrts_sqrt32(float x)
{
    return sqrt(x);
}
static inline float futrts_exp32(float x)
{
    return exp(x);
}
static inline float futrts_cos32(float x)
{
    return cos(x);
}
static inline float futrts_sin32(float x)
{
    return sin(x);
}
static inline float futrts_tan32(float x)
{
    return tan(x);
}
static inline float futrts_acos32(float x)
{
    return acos(x);
}
static inline float futrts_asin32(float x)
{
    return asin(x);
}
static inline float futrts_atan32(float x)
{
    return atan(x);
}
static inline float futrts_atan2_32(float x, float y)
{
    return atan2(x, y);
}
static inline float futrts_round32(float x)
{
    return rint(x);
}
static inline char futrts_isnan32(float x)
{
    return isnan(x);
}
static inline char futrts_isinf32(float x)
{
    return isinf(x);
}
static inline int32_t futrts_to_bits32(float x)
{
    union {
        float f;
        int32_t t;
    } p;
    
    p.f = x;
    return p.t;
}
static inline float futrts_from_bits32(int32_t x)
{
    union {
        int32_t f;
        float t;
    } p;
    
    p.f = x;
    return p.t;
}
static inline double futrts_log64(double x)
{
    return log(x);
}
static inline double futrts_log2_64(double x)
{
    return log2(x);
}
static inline double futrts_log10_64(double x)
{
    return log10(x);
}
static inline double futrts_sqrt64(double x)
{
    return sqrt(x);
}
static inline double futrts_exp64(double x)
{
    return exp(x);
}
static inline double futrts_cos64(double x)
{
    return cos(x);
}
static inline double futrts_sin64(double x)
{
    return sin(x);
}
static inline double futrts_tan64(double x)
{
    return tan(x);
}
static inline double futrts_acos64(double x)
{
    return acos(x);
}
static inline double futrts_asin64(double x)
{
    return asin(x);
}
static inline double futrts_atan64(double x)
{
    return atan(x);
}
static inline double futrts_atan2_64(double x, double y)
{
    return atan2(x, y);
}
static inline double futrts_round64(double x)
{
    return rint(x);
}
static inline char futrts_isnan64(double x)
{
    return isnan(x);
}
static inline char futrts_isinf64(double x)
{
    return isinf(x);
}
static inline int64_t futrts_to_bits64(double x)
{
    union {
        double f;
        int64_t t;
    } p;
    
    p.f = x;
    return p.t;
}
static inline double futrts_from_bits64(int64_t x)
{
    union {
        int64_t f;
        double t;
    } p;
    
    p.f = x;
    return p.t;
}
static int futrts_sum(struct futhark_cpu_context *ctx, double *out_scalar_out_4971,
                      int64_t col_mem_sizze_4951, struct memblock col_mem_4952,
                      int32_t sizze_4841)
{
    double scalar_out_4953;
    double res_4843;
    double redout_4946 = 0.0;
    
    for (int32_t i_4947 = 0; i_4947 < sizze_4841; i_4947++) {
        double x_4847 = *(double *) &col_mem_4952.mem[i_4947 * 8];
        double res_4846 = x_4847 + redout_4946;
        double redout_tmp_4954 = res_4846;
        
        redout_4946 = redout_tmp_4954;
    }
    res_4843 = redout_4946;
    scalar_out_4953 = res_4843;
    *out_scalar_out_4971 = scalar_out_4953;
    return 0;
}
static int futrts_mean(struct futhark_cpu_context *ctx, double *out_scalar_out_4972,
                       int64_t col_mem_sizze_4951, struct memblock col_mem_4952,
                       int32_t sizze_4848)
{
    double scalar_out_4955;
    double res_4850;
    double redout_4946 = 0.0;
    
    for (int32_t i_4947 = 0; i_4947 < sizze_4848; i_4947++) {
        double x_4854 = *(double *) &col_mem_4952.mem[i_4947 * 8];
        double res_4853 = x_4854 + redout_4946;
        double redout_tmp_4956 = res_4853;
        
        redout_4946 = redout_tmp_4956;
    }
    res_4850 = redout_4946;
    
    double res_4855 = sitofp_i32_f64(sizze_4848);
    double res_4856 = res_4850 / res_4855;
    
    scalar_out_4955 = res_4856;
    *out_scalar_out_4972 = scalar_out_4955;
    return 0;
}
static int futrts_variance(struct futhark_cpu_context *ctx,
                           double *out_scalar_out_4973,
                           int64_t values_mem_sizze_4951,
                           struct memblock values_mem_4952, int32_t sizze_4857)
{
    double scalar_out_4957;
    double res_4859 = sitofp_i32_f64(sizze_4857);
    double res_4860;
    double redout_4946 = 0.0;
    
    for (int32_t i_4947 = 0; i_4947 < sizze_4857; i_4947++) {
        double x_4864 = *(double *) &values_mem_4952.mem[i_4947 * 8];
        double res_4863 = x_4864 + redout_4946;
        double redout_tmp_4958 = res_4863;
        
        redout_4946 = redout_tmp_4958;
    }
    res_4860 = redout_4946;
    
    double res_4865 = res_4860 / res_4859;
    double res_4866;
    double redout_4948 = 0.0;
    
    for (int32_t i_4949 = 0; i_4949 < sizze_4857; i_4949++) {
        double x_4870 = *(double *) &values_mem_4952.mem[i_4949 * 8];
        double res_4871 = x_4870 - res_4865;
        double res_4872 = res_4871 * res_4871;
        double res_4869 = res_4872 + redout_4948;
        double redout_tmp_4959 = res_4869;
        
        redout_4948 = redout_tmp_4959;
    }
    res_4866 = redout_4948;
    
    double y_4873 = res_4859 - 1.0;
    double res_4874 = res_4866 / y_4873;
    
    scalar_out_4957 = res_4874;
    *out_scalar_out_4973 = scalar_out_4957;
    return 0;
}
static int futrts_skew(struct futhark_cpu_context *ctx, double *out_scalar_out_4974,
                       int64_t values_mem_sizze_4951,
                       struct memblock values_mem_4952, int32_t sizze_4875)
{
    double scalar_out_4960;
    double res_4877 = sitofp_i32_f64(sizze_4875);
    double res_4878;
    double redout_4946 = 0.0;
    
    for (int32_t i_4947 = 0; i_4947 < sizze_4875; i_4947++) {
        double x_4882 = *(double *) &values_mem_4952.mem[i_4947 * 8];
        double res_4881 = x_4882 + redout_4946;
        double redout_tmp_4961 = res_4881;
        
        redout_4946 = redout_tmp_4961;
    }
    res_4878 = redout_4946;
    
    double res_4883 = res_4878 / res_4877;
    double res_4884;
    double res_4885;
    double redout_4948;
    double redout_4949;
    
    redout_4948 = 0.0;
    redout_4949 = 0.0;
    for (int32_t i_4950 = 0; i_4950 < sizze_4875; i_4950++) {
        double x_4892 = *(double *) &values_mem_4952.mem[i_4950 * 8];
        double res_4893 = x_4892 - res_4883;
        double res_4894 = res_4893 * res_4893;
        double res_4895 = res_4893 * res_4894;
        double res_4890 = res_4894 + redout_4948;
        double res_4891 = res_4895 + redout_4949;
        double redout_tmp_4962 = res_4890;
        double redout_tmp_4963;
        
        redout_tmp_4963 = res_4891;
        redout_4948 = redout_tmp_4962;
        redout_4949 = redout_tmp_4963;
    }
    res_4884 = redout_4948;
    res_4885 = redout_4949;
    
    double res_4896;
    
    res_4896 = futrts_sqrt64(res_4884);
    
    double res_4897;
    
    res_4897 = futrts_sqrt64(res_4877);
    
    double x_4898 = res_4885 * res_4897;
    double x_4899 = res_4896 * res_4896;
    double y_4900 = res_4896 * x_4899;
    double res_4901 = x_4898 / y_4900;
    
    scalar_out_4960 = res_4901;
    *out_scalar_out_4974 = scalar_out_4960;
    return 0;
}
static int futrts_kurtosis(struct futhark_cpu_context *ctx,
                           double *out_scalar_out_4975,
                           int64_t values_mem_sizze_4951,
                           struct memblock values_mem_4952, int32_t sizze_4902)
{
    double scalar_out_4964;
    double res_4904 = sitofp_i32_f64(sizze_4902);
    double res_4905;
    double redout_4946 = 0.0;
    
    for (int32_t i_4947 = 0; i_4947 < sizze_4902; i_4947++) {
        double x_4909 = *(double *) &values_mem_4952.mem[i_4947 * 8];
        double res_4908 = x_4909 + redout_4946;
        double redout_tmp_4965 = res_4908;
        
        redout_4946 = redout_tmp_4965;
    }
    res_4905 = redout_4946;
    
    double res_4910 = res_4905 / res_4904;
    double res_4911;
    double res_4912;
    double redout_4948;
    double redout_4949;
    
    redout_4948 = 0.0;
    redout_4949 = 0.0;
    for (int32_t i_4950 = 0; i_4950 < sizze_4902; i_4950++) {
        double x_4919 = *(double *) &values_mem_4952.mem[i_4950 * 8];
        double res_4920 = x_4919 - res_4910;
        double res_4921 = res_4920 * res_4920;
        double x_4922 = res_4920 * res_4921;
        double res_4923 = res_4920 * x_4922;
        double res_4917 = res_4921 + redout_4948;
        double res_4918 = res_4923 + redout_4949;
        double redout_tmp_4966 = res_4917;
        double redout_tmp_4967;
        
        redout_tmp_4967 = res_4918;
        redout_4948 = redout_tmp_4966;
        redout_4949 = redout_tmp_4967;
    }
    res_4911 = redout_4948;
    res_4912 = redout_4949;
    
    double x_4924 = res_4904 * res_4912;
    double y_4925 = res_4911 * res_4911;
    double res_4926 = x_4924 / y_4925;
    
    scalar_out_4964 = res_4926;
    *out_scalar_out_4975 = scalar_out_4964;
    return 0;
}
static int futrts_stddev(struct futhark_cpu_context *ctx,
                         double *out_scalar_out_4976,
                         int64_t values_mem_sizze_4951,
                         struct memblock values_mem_4952, int32_t sizze_4927)
{
    double scalar_out_4968;
    double res_4929 = sitofp_i32_f64(sizze_4927);
    double res_4930;
    double redout_4946 = 0.0;
    
    for (int32_t i_4947 = 0; i_4947 < sizze_4927; i_4947++) {
        double x_4934 = *(double *) &values_mem_4952.mem[i_4947 * 8];
        double res_4933 = x_4934 + redout_4946;
        double redout_tmp_4969 = res_4933;
        
        redout_4946 = redout_tmp_4969;
    }
    res_4930 = redout_4946;
    
    double res_4935 = res_4930 / res_4929;
    double res_4936;
    double redout_4948 = 0.0;
    
    for (int32_t i_4949 = 0; i_4949 < sizze_4927; i_4949++) {
        double x_4940 = *(double *) &values_mem_4952.mem[i_4949 * 8];
        double res_4941 = x_4940 - res_4935;
        double res_4942 = res_4941 * res_4941;
        double res_4939 = res_4942 + redout_4948;
        double redout_tmp_4970 = res_4939;
        
        redout_4948 = redout_tmp_4970;
    }
    res_4936 = redout_4948;
    
    double y_4943 = res_4929 - 1.0;
    double res_4944 = res_4936 / y_4943;
    double res_4945;
    
    res_4945 = futrts_sqrt64(res_4944);
    scalar_out_4968 = res_4945;
    *out_scalar_out_4976 = scalar_out_4968;
    return 0;
}
struct futhark_cpu_f64_1d {
    struct memblock mem;
    int64_t shape[1];
} ;
struct futhark_cpu_f64_1d *futhark_cpu_new_f64_1d(struct futhark_cpu_context *ctx,
                                          double *data, int dim0)
{
    struct futhark_cpu_f64_1d *arr = malloc(sizeof(struct futhark_cpu_f64_1d));
    
    if (arr == NULL)
        return NULL;
    lock_lock(&ctx->lock);
    arr->mem.references = NULL;
    memblock_alloc(ctx, &arr->mem, dim0 * sizeof(double), "arr->mem");
    arr->shape[0] = dim0;
    memmove(arr->mem.mem + 0, data + 0, dim0 * sizeof(double));
    lock_unlock(&ctx->lock);
    return arr;
}
struct futhark_cpu_f64_1d *futhark_cpu_new_raw_f64_1d(struct futhark_cpu_context *ctx,
                                              char *data, int offset, int dim0)
{
    struct futhark_cpu_f64_1d *arr = malloc(sizeof(struct futhark_cpu_f64_1d));
    
    if (arr == NULL)
        return NULL;
    lock_lock(&ctx->lock);
    arr->mem.references = NULL;
    memblock_alloc(ctx, &arr->mem, dim0 * sizeof(double), "arr->mem");
    arr->shape[0] = dim0;
    memmove(arr->mem.mem + 0, data + offset, dim0 * sizeof(double));
    lock_unlock(&ctx->lock);
    return arr;
}
int futhark_cpu_free_f64_1d(struct futhark_cpu_context *ctx, struct futhark_cpu_f64_1d *arr)
{
    lock_lock(&ctx->lock);
    memblock_unref(ctx, &arr->mem, "arr->mem");
    lock_unlock(&ctx->lock);
    free(arr);
    return 0;
}
int futhark_cpu_values_f64_1d(struct futhark_cpu_context *ctx,
                          struct futhark_cpu_f64_1d *arr, double *data)
{
    lock_lock(&ctx->lock);
    memmove(data + 0, arr->mem.mem + 0, arr->shape[0] * sizeof(double));
    lock_unlock(&ctx->lock);
    return 0;
}
char *futhark_cpu_values_raw_f64_1d(struct futhark_cpu_context *ctx,
                                struct futhark_cpu_f64_1d *arr)
{
    return arr->mem.mem;
}
int64_t *futhark_cpu_shape_f64_1d(struct futhark_cpu_context *ctx,
                              struct futhark_cpu_f64_1d *arr)
{
    return arr->shape;
}
int futhark_cpu_entry_sum(struct futhark_cpu_context *ctx, double *out0, const
                      struct futhark_cpu_f64_1d *in0)
{
    int64_t col_mem_sizze_4951;
    struct memblock col_mem_4952;
    
    col_mem_4952.references = NULL;
    
    int32_t sizze_4841;
    double scalar_out_4953;
    
    lock_lock(&ctx->lock);
    col_mem_4952 = in0->mem;
    col_mem_sizze_4951 = in0->mem.size;
    sizze_4841 = in0->shape[0];
    
    int ret = futrts_sum(ctx, &scalar_out_4953, col_mem_sizze_4951,
                         col_mem_4952, sizze_4841);
    
    if (ret == 0) {
        *out0 = scalar_out_4953;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_cpu_entry_mean(struct futhark_cpu_context *ctx, double *out0, const
                       struct futhark_cpu_f64_1d *in0)
{
    int64_t col_mem_sizze_4951;
    struct memblock col_mem_4952;
    
    col_mem_4952.references = NULL;
    
    int32_t sizze_4848;
    double scalar_out_4955;
    
    lock_lock(&ctx->lock);
    col_mem_4952 = in0->mem;
    col_mem_sizze_4951 = in0->mem.size;
    sizze_4848 = in0->shape[0];
    
    int ret = futrts_mean(ctx, &scalar_out_4955, col_mem_sizze_4951,
                          col_mem_4952, sizze_4848);
    
    if (ret == 0) {
        *out0 = scalar_out_4955;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_cpu_entry_variance(struct futhark_cpu_context *ctx, double *out0, const
                           struct futhark_cpu_f64_1d *in0)
{
    int64_t values_mem_sizze_4951;
    struct memblock values_mem_4952;
    
    values_mem_4952.references = NULL;
    
    int32_t sizze_4857;
    double scalar_out_4957;
    
    lock_lock(&ctx->lock);
    values_mem_4952 = in0->mem;
    values_mem_sizze_4951 = in0->mem.size;
    sizze_4857 = in0->shape[0];
    
    int ret = futrts_variance(ctx, &scalar_out_4957, values_mem_sizze_4951,
                              values_mem_4952, sizze_4857);
    
    if (ret == 0) {
        *out0 = scalar_out_4957;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_cpu_entry_skew(struct futhark_cpu_context *ctx, double *out0, const
                       struct futhark_cpu_f64_1d *in0)
{
    int64_t values_mem_sizze_4951;
    struct memblock values_mem_4952;
    
    values_mem_4952.references = NULL;
    
    int32_t sizze_4875;
    double scalar_out_4960;
    
    lock_lock(&ctx->lock);
    values_mem_4952 = in0->mem;
    values_mem_sizze_4951 = in0->mem.size;
    sizze_4875 = in0->shape[0];
    
    int ret = futrts_skew(ctx, &scalar_out_4960, values_mem_sizze_4951,
                          values_mem_4952, sizze_4875);
    
    if (ret == 0) {
        *out0 = scalar_out_4960;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_cpu_entry_kurtosis(struct futhark_cpu_context *ctx, double *out0, const
                           struct futhark_cpu_f64_1d *in0)
{
    int64_t values_mem_sizze_4951;
    struct memblock values_mem_4952;
    
    values_mem_4952.references = NULL;
    
    int32_t sizze_4902;
    double scalar_out_4964;
    
    lock_lock(&ctx->lock);
    values_mem_4952 = in0->mem;
    values_mem_sizze_4951 = in0->mem.size;
    sizze_4902 = in0->shape[0];
    
    int ret = futrts_kurtosis(ctx, &scalar_out_4964, values_mem_sizze_4951,
                              values_mem_4952, sizze_4902);
    
    if (ret == 0) {
        *out0 = scalar_out_4964;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_cpu_entry_stddev(struct futhark_cpu_context *ctx, double *out0, const
                         struct futhark_cpu_f64_1d *in0)
{
    int64_t values_mem_sizze_4951;
    struct memblock values_mem_4952;
    
    values_mem_4952.references = NULL;
    
    int32_t sizze_4927;
    double scalar_out_4968;
    
    lock_lock(&ctx->lock);
    values_mem_4952 = in0->mem;
    values_mem_sizze_4951 = in0->mem.size;
    sizze_4927 = in0->shape[0];
    
    int ret = futrts_stddev(ctx, &scalar_out_4968, values_mem_sizze_4951,
                            values_mem_4952, sizze_4927);
    
    if (ret == 0) {
        *out0 = scalar_out_4968;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
