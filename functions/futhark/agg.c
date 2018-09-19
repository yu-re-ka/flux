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

/* The simple OpenCL runtime framework used by Futhark. */

#define CL_USE_DEPRECATED_OPENCL_1_2_APIS

#ifdef __APPLE__
  #include <OpenCL/cl.h>
#else
  #include <CL/cl.h>
#endif

#define OPENCL_SUCCEED(e) opencl_succeed(e, #e, __FILE__, __LINE__)

struct opencl_config {
  int debugging;
  int logging;
  int preferred_device_num;
  const char *preferred_platform;
  const char *preferred_device;

  const char* dump_program_to;
  const char* load_program_from;

  size_t default_group_size;
  size_t default_num_groups;
  size_t default_tile_size;
  size_t default_threshold;
  size_t transpose_block_dim;

  int default_group_size_changed;
  int default_tile_size_changed;

  int num_sizes;
  const char **size_names;
  size_t *size_values;
  const char **size_classes;
  const char **size_entry_points;
};

void opencl_config_init(struct opencl_config *cfg,
                        int num_sizes,
                        const char *size_names[],
                        size_t *size_values,
                        const char *size_classes[],
                        const char *size_entry_points[]) {
  cfg->debugging = 0;
  cfg->logging = 0;
  cfg->preferred_device_num = 0;
  cfg->preferred_platform = "";
  cfg->preferred_device = "";
  cfg->dump_program_to = NULL;
  cfg->load_program_from = NULL;

  cfg->default_group_size = 256;
  cfg->default_num_groups = 128;
  cfg->default_tile_size = 32;
  cfg->default_threshold = 32*1024;
  cfg->transpose_block_dim = 16;

  cfg->default_group_size_changed = 0;
  cfg->default_tile_size_changed = 0;

  cfg->num_sizes = num_sizes;
  cfg->size_names = size_names;
  cfg->size_values = size_values;
  cfg->size_classes = size_classes;
  cfg->size_entry_points = size_entry_points;
}

/* An entry in the free list.  May be invalid, to avoid having to
   deallocate entries as soon as they are removed.  There is also a
   tag, to help with memory reuse. */
struct opencl_free_list_entry {
  size_t size;
  cl_mem mem;
  const char *tag;
  unsigned char valid;
};

struct opencl_free_list {
  struct opencl_free_list_entry *entries; // Pointer to entries.
  int capacity;                           // Number of entries.
  int used;                               // Number of valid entries.
};

void free_list_init(struct opencl_free_list *l) {
  l->capacity = 30; // Picked arbitrarily.
  l->used = 0;
  l->entries = malloc(sizeof(struct opencl_free_list_entry) * l->capacity);
  for (int i = 0; i < l->capacity; i++) {
    l->entries[i].valid = 0;
  }
}

/* Remove invalid entries from the free list. */
void free_list_pack(struct opencl_free_list *l) {
  int p = 0;
  for (int i = 0; i < l->capacity; i++) {
    if (l->entries[i].valid) {
      l->entries[p] = l->entries[i];
      p++;
    }
  }
  // Now p == l->used.
  l->entries = realloc(l->entries, l->used * sizeof(struct opencl_free_list_entry));
  l->capacity = l->used;
}

void free_list_destroy(struct opencl_free_list *l) {
  assert(l->used == 0);
  free(l->entries);
}

int free_list_find_invalid(struct opencl_free_list *l) {
  int i;
  for (i = 0; i < l->capacity; i++) {
    if (!l->entries[i].valid) {
      break;
    }
  }
  return i;
}

void free_list_insert(struct opencl_free_list *l, size_t size, cl_mem mem, const char *tag) {
  int i = free_list_find_invalid(l);

  if (i == l->capacity) {
    // List is full; so we have to grow it.
    int new_capacity = l->capacity * 2 * sizeof(struct opencl_free_list_entry);
    l->entries = realloc(l->entries, new_capacity);
    for (int j = 0; j < l->capacity; j++) {
      l->entries[j+l->capacity].valid = 0;
    }
    l->capacity *= 2;
  }

  // Now 'i' points to the first invalid entry.
  l->entries[i].valid = 1;
  l->entries[i].size = size;
  l->entries[i].mem = mem;
  l->entries[i].tag = tag;

  l->used++;
}

/* Find and remove a memory block of at least the desired size and
   tag.  Returns 0 on success.  */
int free_list_find(struct opencl_free_list *l, const char *tag, size_t *size_out, cl_mem *mem_out) {
  int i;
  for (i = 0; i < l->capacity; i++) {
    if (l->entries[i].valid && l->entries[i].tag == tag) {
      l->entries[i].valid = 0;
      *size_out = l->entries[i].size;
      *mem_out = l->entries[i].mem;
      l->used--;
      return 0;
    }
  }

  return 1;
}

/* Remove the first block in the free list.  Returns 0 if a block was
   removed, and nonzero if the free list was already empty. */
int free_list_first(struct opencl_free_list *l, cl_mem *mem_out) {
  for (int i = 0; i < l->capacity; i++) {
    if (l->entries[i].valid) {
      l->entries[i].valid = 0;
      *mem_out = l->entries[i].mem;
      l->used--;
      return 0;
    }
  }

  return 1;
}

struct opencl_context {
  cl_device_id device;
  cl_context ctx;
  cl_command_queue queue;

  struct opencl_config cfg;

  struct opencl_free_list free_list;

  size_t max_group_size;
  size_t max_num_groups;
  size_t max_tile_size;
  size_t max_threshold;

  size_t lockstep_width;
};

struct opencl_device_option {
  cl_platform_id platform;
  cl_device_id device;
  cl_device_type device_type;
  char *platform_name;
  char *device_name;
};

/* This function must be defined by the user.  It is invoked by
   setup_opencl() after the platform and device has been found, but
   before the program is loaded.  Its intended use is to tune
   constants based on the selected platform and device. */
static void post_opencl_setup(struct opencl_context*, struct opencl_device_option*);

static char *strclone(const char *str) {
  size_t size = strlen(str) + 1;
  char *copy = malloc(size);
  if (copy == NULL) {
    return NULL;
  }

  memcpy(copy, str, size);
  return copy;
}

static const char* opencl_error_string(unsigned int err)
{
    switch (err) {
        case CL_SUCCESS:                            return "Success!";
        case CL_DEVICE_NOT_FOUND:                   return "Device not found.";
        case CL_DEVICE_NOT_AVAILABLE:               return "Device not available";
        case CL_COMPILER_NOT_AVAILABLE:             return "Compiler not available";
        case CL_MEM_OBJECT_ALLOCATION_FAILURE:      return "Memory object allocation failure";
        case CL_OUT_OF_RESOURCES:                   return "Out of resources";
        case CL_OUT_OF_HOST_MEMORY:                 return "Out of host memory";
        case CL_PROFILING_INFO_NOT_AVAILABLE:       return "Profiling information not available";
        case CL_MEM_COPY_OVERLAP:                   return "Memory copy overlap";
        case CL_IMAGE_FORMAT_MISMATCH:              return "Image format mismatch";
        case CL_IMAGE_FORMAT_NOT_SUPPORTED:         return "Image format not supported";
        case CL_BUILD_PROGRAM_FAILURE:              return "Program build failure";
        case CL_MAP_FAILURE:                        return "Map failure";
        case CL_INVALID_VALUE:                      return "Invalid value";
        case CL_INVALID_DEVICE_TYPE:                return "Invalid device type";
        case CL_INVALID_PLATFORM:                   return "Invalid platform";
        case CL_INVALID_DEVICE:                     return "Invalid device";
        case CL_INVALID_CONTEXT:                    return "Invalid context";
        case CL_INVALID_QUEUE_PROPERTIES:           return "Invalid queue properties";
        case CL_INVALID_COMMAND_QUEUE:              return "Invalid command queue";
        case CL_INVALID_HOST_PTR:                   return "Invalid host pointer";
        case CL_INVALID_MEM_OBJECT:                 return "Invalid memory object";
        case CL_INVALID_IMAGE_FORMAT_DESCRIPTOR:    return "Invalid image format descriptor";
        case CL_INVALID_IMAGE_SIZE:                 return "Invalid image size";
        case CL_INVALID_SAMPLER:                    return "Invalid sampler";
        case CL_INVALID_BINARY:                     return "Invalid binary";
        case CL_INVALID_BUILD_OPTIONS:              return "Invalid build options";
        case CL_INVALID_PROGRAM:                    return "Invalid program";
        case CL_INVALID_PROGRAM_EXECUTABLE:         return "Invalid program executable";
        case CL_INVALID_KERNEL_NAME:                return "Invalid kernel name";
        case CL_INVALID_KERNEL_DEFINITION:          return "Invalid kernel definition";
        case CL_INVALID_KERNEL:                     return "Invalid kernel";
        case CL_INVALID_ARG_INDEX:                  return "Invalid argument index";
        case CL_INVALID_ARG_VALUE:                  return "Invalid argument value";
        case CL_INVALID_ARG_SIZE:                   return "Invalid argument size";
        case CL_INVALID_KERNEL_ARGS:                return "Invalid kernel arguments";
        case CL_INVALID_WORK_DIMENSION:             return "Invalid work dimension";
        case CL_INVALID_WORK_GROUP_SIZE:            return "Invalid work group size";
        case CL_INVALID_WORK_ITEM_SIZE:             return "Invalid work item size";
        case CL_INVALID_GLOBAL_OFFSET:              return "Invalid global offset";
        case CL_INVALID_EVENT_WAIT_LIST:            return "Invalid event wait list";
        case CL_INVALID_EVENT:                      return "Invalid event";
        case CL_INVALID_OPERATION:                  return "Invalid operation";
        case CL_INVALID_GL_OBJECT:                  return "Invalid OpenGL object";
        case CL_INVALID_BUFFER_SIZE:                return "Invalid buffer size";
        case CL_INVALID_MIP_LEVEL:                  return "Invalid mip-map level";
        default:                                    return "Unknown";
    }
}

static void opencl_succeed(unsigned int ret,
                           const char *call,
                           const char *file,
                           int line) {
  if (ret != CL_SUCCESS) {
    panic(-1, "%s:%d: OpenCL call\n  %s\nfailed with error code %d (%s)\n",
          file, line, call, ret, opencl_error_string(ret));
  }
}

void set_preferred_platform(struct opencl_config *cfg, const char *s) {
  cfg->preferred_platform = s;
}

void set_preferred_device(struct opencl_config *cfg, const char *s) {
  int x = 0;
  if (*s == '#') {
    s++;
    while (isdigit(*s)) {
      x = x * 10 + (*s++)-'0';
    }
    // Skip trailing spaces.
    while (isspace(*s)) {
      s++;
    }
  }
  cfg->preferred_device = s;
  cfg->preferred_device_num = x;
}

static char* opencl_platform_info(cl_platform_id platform,
                                  cl_platform_info param) {
  size_t req_bytes;
  char *info;

  OPENCL_SUCCEED(clGetPlatformInfo(platform, param, 0, NULL, &req_bytes));

  info = malloc(req_bytes);

  OPENCL_SUCCEED(clGetPlatformInfo(platform, param, req_bytes, info, NULL));

  return info;
}

static char* opencl_device_info(cl_device_id device,
                                cl_device_info param) {
  size_t req_bytes;
  char *info;

  OPENCL_SUCCEED(clGetDeviceInfo(device, param, 0, NULL, &req_bytes));

  info = malloc(req_bytes);

  OPENCL_SUCCEED(clGetDeviceInfo(device, param, req_bytes, info, NULL));

  return info;
}

static void opencl_all_device_options(struct opencl_device_option **devices_out,
                                      size_t *num_devices_out) {
  size_t num_devices = 0, num_devices_added = 0;

  cl_platform_id *all_platforms;
  cl_uint *platform_num_devices;

  cl_uint num_platforms;

  // Find the number of platforms.
  OPENCL_SUCCEED(clGetPlatformIDs(0, NULL, &num_platforms));

  // Make room for them.
  all_platforms = calloc(num_platforms, sizeof(cl_platform_id));
  platform_num_devices = calloc(num_platforms, sizeof(cl_uint));

  // Fetch all the platforms.
  OPENCL_SUCCEED(clGetPlatformIDs(num_platforms, all_platforms, NULL));

  // Count the number of devices for each platform, as well as the
  // total number of devices.
  for (cl_uint i = 0; i < num_platforms; i++) {
    if (clGetDeviceIDs(all_platforms[i], CL_DEVICE_TYPE_ALL,
                       0, NULL, &platform_num_devices[i]) == CL_SUCCESS) {
      num_devices += platform_num_devices[i];
    } else {
      platform_num_devices[i] = 0;
    }
  }

  // Make room for all the device options.
  struct opencl_device_option *devices =
    calloc(num_devices, sizeof(struct opencl_device_option));

  // Loop through the platforms, getting information about their devices.
  for (cl_uint i = 0; i < num_platforms; i++) {
    cl_platform_id platform = all_platforms[i];
    cl_uint num_platform_devices = platform_num_devices[i];

    if (num_platform_devices == 0) {
      continue;
    }

    char *platform_name = opencl_platform_info(platform, CL_PLATFORM_NAME);
    cl_device_id *platform_devices =
      calloc(num_platform_devices, sizeof(cl_device_id));

    // Fetch all the devices.
    OPENCL_SUCCEED(clGetDeviceIDs(platform, CL_DEVICE_TYPE_ALL,
                                  num_platform_devices, platform_devices, NULL));

    // Loop through the devices, adding them to the devices array.
    for (cl_uint i = 0; i < num_platform_devices; i++) {
      char *device_name = opencl_device_info(platform_devices[i], CL_DEVICE_NAME);
      devices[num_devices_added].platform = platform;
      devices[num_devices_added].device = platform_devices[i];
      OPENCL_SUCCEED(clGetDeviceInfo(platform_devices[i], CL_DEVICE_TYPE,
                                     sizeof(cl_device_type),
                                     &devices[num_devices_added].device_type,
                                     NULL));
      // We don't want the structs to share memory, so copy the platform name.
      // Each device name is already unique.
      devices[num_devices_added].platform_name = strclone(platform_name);
      devices[num_devices_added].device_name = device_name;
      num_devices_added++;
    }
    free(platform_devices);
    free(platform_name);
  }
  free(all_platforms);
  free(platform_num_devices);

  *devices_out = devices;
  *num_devices_out = num_devices;
}

static int is_blacklisted(const char *platform_name, const char *device_name,
                          const struct opencl_config *cfg) {
  if (strcmp(cfg->preferred_platform, "") != 0 ||
      strcmp(cfg->preferred_device, "") != 0) {
    return 0;
  } else if (strstr(platform_name, "Apple") != NULL &&
             strstr(device_name, "Intel(R) Core(TM)") != NULL) {
    return 1;
  } else {
    return 0;
  }
}

static struct opencl_device_option get_preferred_device(const struct opencl_config *cfg) {
  struct opencl_device_option *devices;
  size_t num_devices;

  opencl_all_device_options(&devices, &num_devices);

  int num_device_matches = 0;

  for (size_t i = 0; i < num_devices; i++) {
    struct opencl_device_option device = devices[i];
    if (!is_blacklisted(device.platform_name, device.device_name, cfg) &&
        strstr(device.platform_name, cfg->preferred_platform) != NULL &&
        strstr(device.device_name, cfg->preferred_device) != NULL &&
        num_device_matches++ == cfg->preferred_device_num) {
      // Free all the platform and device names, except the ones we have chosen.
      for (size_t j = 0; j < num_devices; j++) {
        if (j != i) {
          free(devices[j].platform_name);
          free(devices[j].device_name);
        }
      }
      free(devices);
      return device;
    }
  }

  panic(1, "Could not find acceptable OpenCL device.\n");
  exit(1); // Never reached
}

static void describe_device_option(struct opencl_device_option device) {
  fprintf(stderr, "Using platform: %s\n", device.platform_name);
  fprintf(stderr, "Using device: %s\n", device.device_name);
}

static cl_build_status build_opencl_program(cl_program program, cl_device_id device, const char* options) {
  cl_int ret_val = clBuildProgram(program, 1, &device, options, NULL, NULL);

  // Avoid termination due to CL_BUILD_PROGRAM_FAILURE
  if (ret_val != CL_SUCCESS && ret_val != CL_BUILD_PROGRAM_FAILURE) {
    assert(ret_val == 0);
  }

  cl_build_status build_status;
  ret_val = clGetProgramBuildInfo(program,
                                  device,
                                  CL_PROGRAM_BUILD_STATUS,
                                  sizeof(cl_build_status),
                                  &build_status,
                                  NULL);
  assert(ret_val == 0);

  if (build_status != CL_SUCCESS) {
    char *build_log;
    size_t ret_val_size;
    ret_val = clGetProgramBuildInfo(program, device, CL_PROGRAM_BUILD_LOG, 0, NULL, &ret_val_size);
    assert(ret_val == 0);

    build_log = malloc(ret_val_size+1);
    clGetProgramBuildInfo(program, device, CL_PROGRAM_BUILD_LOG, ret_val_size, build_log, NULL);
    assert(ret_val == 0);

    // The spec technically does not say whether the build log is zero-terminated, so let's be careful.
    build_log[ret_val_size] = '\0';

    fprintf(stderr, "Build log:\n%s\n", build_log);

    free(build_log);
  }

  return build_status;
}

/* Fields in a bitmask indicating which types we must be sure are
   available. */
enum opencl_required_type { OPENCL_F64 = 1 };

// We take as input several strings representing the program, because
// C does not guarantee that the compiler supports particularly large
// literals.  Notably, Visual C has a limit of 2048 characters.  The
// array must be NULL-terminated.
static cl_program setup_opencl_with_command_queue(struct opencl_context *ctx,
                                                  cl_command_queue queue,
                                                  const char *srcs[],
                                                  int required_types) {
  int error;

  ctx->queue = queue;

  OPENCL_SUCCEED(clGetCommandQueueInfo(ctx->queue, CL_QUEUE_CONTEXT, sizeof(cl_context), &ctx->ctx, NULL));

  // Fill out the device info.  This is redundant work if we are
  // called from setup_opencl() (which is the common case), but I
  // doubt it matters much.
  struct opencl_device_option device_option;
  OPENCL_SUCCEED(clGetCommandQueueInfo(ctx->queue, CL_QUEUE_DEVICE,
                                       sizeof(cl_device_id),
                                       &device_option.device,
                                       NULL));
  OPENCL_SUCCEED(clGetDeviceInfo(device_option.device, CL_DEVICE_PLATFORM,
                                 sizeof(cl_platform_id),
                                 &device_option.platform,
                                 NULL));
  OPENCL_SUCCEED(clGetDeviceInfo(device_option.device, CL_DEVICE_TYPE,
                                 sizeof(cl_device_type),
                                 &device_option.device_type,
                                 NULL));
  device_option.platform_name = opencl_platform_info(device_option.platform, CL_PLATFORM_NAME);
  device_option.device_name = opencl_device_info(device_option.device, CL_DEVICE_NAME);

  ctx->device = device_option.device;

  if (required_types & OPENCL_F64) {
    cl_uint supported;
    OPENCL_SUCCEED(clGetDeviceInfo(device_option.device, CL_DEVICE_PREFERRED_VECTOR_WIDTH_DOUBLE,
                                   sizeof(cl_uint), &supported, NULL));
    if (!supported) {
      panic(1, "Program uses double-precision floats, but this is not supported on the chosen device: %s",
            device_option.device_name);
    }
  }

  size_t max_group_size;
  OPENCL_SUCCEED(clGetDeviceInfo(device_option.device, CL_DEVICE_MAX_WORK_GROUP_SIZE,
                                 sizeof(size_t), &max_group_size, NULL));

  size_t max_tile_size = sqrt(max_group_size);

  if (max_group_size < ctx->cfg.default_group_size) {
    if (ctx->cfg.default_group_size_changed) {
      fprintf(stderr, "Note: Device limits default group size to %zu (down from %zu).\n",
              max_group_size, ctx->cfg.default_group_size);
    }
    ctx->cfg.default_group_size = max_group_size;
  }

  if (max_tile_size < ctx->cfg.default_tile_size) {
    if (ctx->cfg.default_tile_size_changed) {
      fprintf(stderr, "Note: Device limits default tile size to %zu (down from %zu).\n",
              max_tile_size, ctx->cfg.default_tile_size);
    }
    ctx->cfg.default_tile_size = max_tile_size;
  }

  ctx->max_group_size = max_group_size;
  ctx->max_tile_size = max_tile_size; // No limit.
  ctx->max_threshold = ctx->max_num_groups = 0; // No limit.

  // Now we go through all the sizes, clamp them to the valid range,
  // or set them to the default.
  for (int i = 0; i < ctx->cfg.num_sizes; i++) {
    const char *size_class = ctx->cfg.size_classes[i];
    size_t *size_value = &ctx->cfg.size_values[i];
    const char* size_name = ctx->cfg.size_names[i];
    size_t max_value, default_value;
    if (strstr(size_class, "group_size") == size_class) {
      max_value = max_group_size;
      default_value = ctx->cfg.default_group_size;
    } else if (strstr(size_class, "num_groups") == size_class) {
      max_value = max_group_size; // Futhark assumes this constraint.
      default_value = ctx->cfg.default_num_groups;
    } else if (strstr(size_class, "tile_size") == size_class) {
      max_value = sqrt(max_group_size);
      default_value = ctx->cfg.default_tile_size;
    } else if (strstr(size_class, "threshold") == size_class) {
      max_value = 0; // No limit.
      default_value = ctx->cfg.default_threshold;
    } else {
      panic(1, "Unknown size class for size '%s': %s\n", size_name, size_class);
    }
    if (*size_value == 0) {
      *size_value = default_value;
    } else if (max_value > 0 && *size_value > max_value) {
      fprintf(stderr, "Note: Device limits %s to %d (down from %d)\n",
              size_name, (int)max_value, (int)*size_value);
      *size_value = max_value;
    }
  }

  // Make sure this function is defined.
  post_opencl_setup(ctx, &device_option);

  if (ctx->cfg.logging) {
    fprintf(stderr, "Lockstep width: %d\n", (int)ctx->lockstep_width);
    fprintf(stderr, "Default group size: %d\n", (int)ctx->cfg.default_group_size);
    fprintf(stderr, "Default number of groups: %d\n", (int)ctx->cfg.default_num_groups);
  }

  char *fut_opencl_src = NULL;
  size_t src_size = 0;

  // Maybe we have to read OpenCL source from somewhere else (used for debugging).
  if (ctx->cfg.load_program_from != NULL) {
    FILE *f = fopen(ctx->cfg.load_program_from, "r");
    assert(f != NULL);
    fseek(f, 0, SEEK_END);
    src_size = ftell(f);
    fseek(f, 0, SEEK_SET);
    fut_opencl_src = malloc(src_size);
    assert(fread(fut_opencl_src, 1, src_size, f) == src_size);
    fclose(f);
  } else {
    // Build the OpenCL program.  First we have to concatenate all the fragments.
    for (const char **src = srcs; src && *src; src++) {
      src_size += strlen(*src);
    }

    fut_opencl_src = malloc(src_size + 1);

    size_t n, i;
    for (i = 0, n = 0; srcs && srcs[i]; i++) {
      strncpy(fut_opencl_src+n, srcs[i], src_size-n);
      n += strlen(srcs[i]);
    }
    fut_opencl_src[src_size] = 0;

  }

  cl_program prog;
  error = 0;
  const char* src_ptr[] = {fut_opencl_src};

  if (ctx->cfg.dump_program_to != NULL) {
    FILE *f = fopen(ctx->cfg.dump_program_to, "w");
    assert(f != NULL);
    fputs(fut_opencl_src, f);
    fclose(f);
  }

  prog = clCreateProgramWithSource(ctx->ctx, 1, src_ptr, &src_size, &error);
  assert(error == 0);

  int compile_opts_size = 1024;
  for (int i = 0; i < ctx->cfg.num_sizes; i++) {
    compile_opts_size += strlen(ctx->cfg.size_names[i]) + 20;
  }
  char *compile_opts = malloc(compile_opts_size);

  int w = snprintf(compile_opts, compile_opts_size,
                   "-DFUT_BLOCK_DIM=%d -DLOCKSTEP_WIDTH=%d ",
                   (int)ctx->cfg.transpose_block_dim,
                   (int)ctx->lockstep_width);

  for (int i = 0; i < ctx->cfg.num_sizes; i++) {
    w += snprintf(compile_opts+w, compile_opts_size-w,
                  "-D%s=%d ", ctx->cfg.size_names[i],
                  (int)ctx->cfg.size_values[i]);
  }

  OPENCL_SUCCEED(build_opencl_program(prog, device_option.device, compile_opts));
  free(compile_opts);
  free(fut_opencl_src);

  return prog;
}

static cl_program setup_opencl(struct opencl_context *ctx,
                               const char *srcs[],
                               int required_types) {

  ctx->lockstep_width = 1;

  free_list_init(&ctx->free_list);

  struct opencl_device_option device_option = get_preferred_device(&ctx->cfg);

  if (ctx->cfg.logging) {
    describe_device_option(device_option);
  }

  // Note that NVIDIA's OpenCL requires the platform property
  cl_context_properties properties[] = {
    CL_CONTEXT_PLATFORM,
    (cl_context_properties)device_option.platform,
    0
  };

  cl_int error;

  ctx->ctx = clCreateContext(properties, 1, &device_option.device, NULL, NULL, &error);
  assert(error == 0);

  cl_command_queue queue = clCreateCommandQueue(ctx->ctx, device_option.device, 0, &error);
  assert(error == 0);

  return setup_opencl_with_command_queue(ctx, queue, srcs, required_types);
}

// Allocate memory from driver. The problem is that OpenCL may perform
// lazy allocation, so we cannot know whether an allocation succeeded
// until the first time we try to use it.  Hence we immediately
// perform a write to see if the allocation succeeded.  This is slow,
// but the assumption is that this operation will be rare (most things
// will go through the free list).
int opencl_alloc_actual(struct opencl_context *ctx, size_t size, cl_mem *mem_out) {
  int error;
  *mem_out = clCreateBuffer(ctx->ctx, CL_MEM_READ_WRITE, size, NULL, &error);

  if (error != CL_SUCCESS) {
    return error;
  }

  int x = 2;
  error = clEnqueueWriteBuffer(ctx->queue, *mem_out, 1, 0, sizeof(x), &x, 0, NULL, NULL);

  // No need to wait for completion here. clWaitForEvents() cannot
  // return mem object allocation failures. This implies that the
  // buffer is faulted onto the device on enqueue. (Observation by
  // Andreas Kloeckner.)

  return error;
}

int opencl_alloc(struct opencl_context *ctx, size_t min_size, const char *tag, cl_mem *mem_out) {
  assert(min_size >= 0);
  if (min_size < sizeof(int)) {
    min_size = sizeof(int);
  }

  size_t size;

  if (free_list_find(&ctx->free_list, tag, &size, mem_out) == 0) {
    // Successfully found a free block.  Is it big enough?
    //
    // FIXME: we might also want to check whether the block is *too
    // big*, to avoid internal fragmentation.  However, this can
    // sharply impact performance on programs where arrays change size
    // frequently.  Fortunately, such allocations are usually fairly
    // short-lived, as they are necessarily within a loop, so the risk
    // of internal fragmentation resulting in an OOM situation is
    // limited.  However, it would be preferable if we could go back
    // and *shrink* oversize allocations when we encounter an OOM
    // condition.  That is technically feasible, since we do not
    // expose OpenCL pointer values directly to the application, but
    // instead rely on a level of indirection.
    if (size >= min_size) {
      return CL_SUCCESS;
    } else {
      // Not just right - free it.
      int error = clReleaseMemObject(*mem_out);
      if (error != CL_SUCCESS) {
        return error;
      }
    }
  }

  // We have to allocate a new block from the driver.  If the
  // allocation does not succeed, then we might be in an out-of-memory
  // situation.  We now start freeing things from the free list until
  // we think we have freed enough that the allocation will succeed.
  // Since we don't know how far the allocation is from fitting, we
  // have to check after every deallocation.  This might be pretty
  // expensive.  Let's hope that this case is hit rarely.

  int error = opencl_alloc_actual(ctx, min_size, mem_out);

  while (error == CL_MEM_OBJECT_ALLOCATION_FAILURE) {
    cl_mem mem;
    if (free_list_first(&ctx->free_list, &mem) == 0) {
      error = clReleaseMemObject(mem);
      if (error != CL_SUCCESS) {
        return error;
      }
    } else {
      break;
    }
    error = opencl_alloc_actual(ctx, min_size, mem_out);
  }

  return error;
}

int opencl_free(struct opencl_context *ctx, cl_mem mem, const char *tag) {
  size_t size;
  cl_mem existing_mem;

  // If there is already a block with this tag, then remove it.
  if (free_list_find(&ctx->free_list, tag, &size, &existing_mem) == 0) {
    int error = clReleaseMemObject(existing_mem);
    if (error != CL_SUCCESS) {
      return error;
    }
  }

  int error = clGetMemObjectInfo(mem, CL_MEM_SIZE, sizeof(size_t), &size, NULL);

  if (error == CL_SUCCESS) {
    free_list_insert(&ctx->free_list, size, mem, tag);
  }

  return error;
}

int opencl_free_all(struct opencl_context *ctx) {
  cl_mem mem;
  free_list_pack(&ctx->free_list);
  while (free_list_first(&ctx->free_list, &mem) == 0) {
    int error = clReleaseMemObject(mem);
    if (error != CL_SUCCESS) {
      return error;
    }
  }

  return CL_SUCCESS;
}

const char *opencl_program[] =
           {"#pragma OPENCL EXTENSION cl_clang_storage_class_specifiers : enable\n#pragma OPENCL EXTENSION cl_khr_fp64 : enable\n__kernel void dummy_kernel(__global unsigned char *dummy, int n)\n{\n    const int thread_gid = get_global_id(0);\n    \n    if (thread_gid >= n)\n        return;\n}\ntypedef char int8_t;\ntypedef short int16_t;\ntypedef int int32_t;\ntypedef long int64_t;\ntypedef uchar uint8_t;\ntypedef ushort uint16_t;\ntypedef uint uint32_t;\ntypedef ulong uint64_t;\n#define ALIGNED_LOCAL_MEMORY(m,size) __local unsigned char m[size] __attribute__ ((align))\nstatic inline int8_t add8(int8_t x, int8_t y)\n{\n    return x + y;\n}\nstatic inline int16_t add16(int16_t x, int16_t y)\n{\n    return x + y;\n}\nstatic inline int32_t add32(int32_t x, int32_t y)\n{\n    return x + y;\n}\nstatic inline int64_t add64(int64_t x, int64_t y)\n{\n    return x + y;\n}\nstatic inline int8_t sub8(int8_t x, int8_t y)\n{\n    return x - y;\n}\nstatic inline int16_t sub16(int16_t x, int16_t y)\n{\n    return x - y;\n}\nstatic inline int32_t sub32(int32_t x, int32_t y)\n{\n    return x - y;\n}\nstatic inline int64_t sub64(int64_t x, int64_t y)\n{\n    return x - y;\n}\nstatic inline int8_t mul8(int8_t x, int8_t y)\n{\n    return x * y;\n}\nstatic inline int16_t mul16(int16_t x, int16_t y)\n{\n    return x * y;\n}\nstatic inline int32_t mul32(int32_t x, int32_t y)\n{\n    return x * y;\n}\nstatic inline int64_t mul64(int64_t x, int64_t y)\n{\n    return x * y;\n}\nstatic inline uint8_t udiv8(uint8_t x, uint8_t y)\n{\n    return x / y;\n}\nstatic inline uint16_t udiv16(uint16_t x, uint16_t y)\n{\n    return x / y;\n}\nstatic inline uint32_t udiv32(uint32_t x, uint32_t y)\n{\n    return x / y;\n}\nstatic inline uint64_t udiv64(uint64_t x, uint64_t y)\n{\n    return x / y;\n}\nstatic inline uint8_t umod8(uint8_t x, uint8_t y)\n{\n    return x % y;\n}\nstatic inline uint16_t umod16(uint16_t x, uint16_t y)\n{\n    return x % y;\n}\nstatic inline uint32_t umod32(uint32_t x, uint32_t y)\n{\n    return x % y;\n}\nstatic inline uint64_t umod64(uint64_t x, uint64_t y)\n{\n    return x % y;\n}\ns",
            "tatic inline int8_t sdiv8(int8_t x, int8_t y)\n{\n    int8_t q = x / y;\n    int8_t r = x % y;\n    \n    return q - ((r != 0 && r < 0 != y < 0) ? 1 : 0);\n}\nstatic inline int16_t sdiv16(int16_t x, int16_t y)\n{\n    int16_t q = x / y;\n    int16_t r = x % y;\n    \n    return q - ((r != 0 && r < 0 != y < 0) ? 1 : 0);\n}\nstatic inline int32_t sdiv32(int32_t x, int32_t y)\n{\n    int32_t q = x / y;\n    int32_t r = x % y;\n    \n    return q - ((r != 0 && r < 0 != y < 0) ? 1 : 0);\n}\nstatic inline int64_t sdiv64(int64_t x, int64_t y)\n{\n    int64_t q = x / y;\n    int64_t r = x % y;\n    \n    return q - ((r != 0 && r < 0 != y < 0) ? 1 : 0);\n}\nstatic inline int8_t smod8(int8_t x, int8_t y)\n{\n    int8_t r = x % y;\n    \n    return r + (r == 0 || (x > 0 && y > 0) || (x < 0 && y < 0) ? 0 : y);\n}\nstatic inline int16_t smod16(int16_t x, int16_t y)\n{\n    int16_t r = x % y;\n    \n    return r + (r == 0 || (x > 0 && y > 0) || (x < 0 && y < 0) ? 0 : y);\n}\nstatic inline int32_t smod32(int32_t x, int32_t y)\n{\n    int32_t r = x % y;\n    \n    return r + (r == 0 || (x > 0 && y > 0) || (x < 0 && y < 0) ? 0 : y);\n}\nstatic inline int64_t smod64(int64_t x, int64_t y)\n{\n    int64_t r = x % y;\n    \n    return r + (r == 0 || (x > 0 && y > 0) || (x < 0 && y < 0) ? 0 : y);\n}\nstatic inline int8_t squot8(int8_t x, int8_t y)\n{\n    return x / y;\n}\nstatic inline int16_t squot16(int16_t x, int16_t y)\n{\n    return x / y;\n}\nstatic inline int32_t squot32(int32_t x, int32_t y)\n{\n    return x / y;\n}\nstatic inline int64_t squot64(int64_t x, int64_t y)\n{\n    return x / y;\n}\nstatic inline int8_t srem8(int8_t x, int8_t y)\n{\n    return x % y;\n}\nstatic inline int16_t srem16(int16_t x, int16_t y)\n{\n    return x % y;\n}\nstatic inline int32_t srem32(int32_t x, int32_t y)\n{\n    return x % y;\n}\nstatic inline int64_t srem64(int64_t x, int64_t y)\n{\n    return x % y;\n}\nstatic inline int8_t smin8(int8_t x, int8_t y)\n{\n    return x < y ? x : y;\n}\nstatic inline int16_t smin16(int16_t x, int16_t y)\n{\n    return x < y ? x : y;\n}\nstatic inline ",
            "int32_t smin32(int32_t x, int32_t y)\n{\n    return x < y ? x : y;\n}\nstatic inline int64_t smin64(int64_t x, int64_t y)\n{\n    return x < y ? x : y;\n}\nstatic inline uint8_t umin8(uint8_t x, uint8_t y)\n{\n    return x < y ? x : y;\n}\nstatic inline uint16_t umin16(uint16_t x, uint16_t y)\n{\n    return x < y ? x : y;\n}\nstatic inline uint32_t umin32(uint32_t x, uint32_t y)\n{\n    return x < y ? x : y;\n}\nstatic inline uint64_t umin64(uint64_t x, uint64_t y)\n{\n    return x < y ? x : y;\n}\nstatic inline int8_t smax8(int8_t x, int8_t y)\n{\n    return x < y ? y : x;\n}\nstatic inline int16_t smax16(int16_t x, int16_t y)\n{\n    return x < y ? y : x;\n}\nstatic inline int32_t smax32(int32_t x, int32_t y)\n{\n    return x < y ? y : x;\n}\nstatic inline int64_t smax64(int64_t x, int64_t y)\n{\n    return x < y ? y : x;\n}\nstatic inline uint8_t umax8(uint8_t x, uint8_t y)\n{\n    return x < y ? y : x;\n}\nstatic inline uint16_t umax16(uint16_t x, uint16_t y)\n{\n    return x < y ? y : x;\n}\nstatic inline uint32_t umax32(uint32_t x, uint32_t y)\n{\n    return x < y ? y : x;\n}\nstatic inline uint64_t umax64(uint64_t x, uint64_t y)\n{\n    return x < y ? y : x;\n}\nstatic inline uint8_t shl8(uint8_t x, uint8_t y)\n{\n    return x << y;\n}\nstatic inline uint16_t shl16(uint16_t x, uint16_t y)\n{\n    return x << y;\n}\nstatic inline uint32_t shl32(uint32_t x, uint32_t y)\n{\n    return x << y;\n}\nstatic inline uint64_t shl64(uint64_t x, uint64_t y)\n{\n    return x << y;\n}\nstatic inline uint8_t lshr8(uint8_t x, uint8_t y)\n{\n    return x >> y;\n}\nstatic inline uint16_t lshr16(uint16_t x, uint16_t y)\n{\n    return x >> y;\n}\nstatic inline uint32_t lshr32(uint32_t x, uint32_t y)\n{\n    return x >> y;\n}\nstatic inline uint64_t lshr64(uint64_t x, uint64_t y)\n{\n    return x >> y;\n}\nstatic inline int8_t ashr8(int8_t x, int8_t y)\n{\n    return x >> y;\n}\nstatic inline int16_t ashr16(int16_t x, int16_t y)\n{\n    return x >> y;\n}\nstatic inline int32_t ashr32(int32_t x, int32_t y)\n{\n    return x >> y;\n}\nstatic inline int64_t ashr64(int64_t x, int64_",
            "t y)\n{\n    return x >> y;\n}\nstatic inline uint8_t and8(uint8_t x, uint8_t y)\n{\n    return x & y;\n}\nstatic inline uint16_t and16(uint16_t x, uint16_t y)\n{\n    return x & y;\n}\nstatic inline uint32_t and32(uint32_t x, uint32_t y)\n{\n    return x & y;\n}\nstatic inline uint64_t and64(uint64_t x, uint64_t y)\n{\n    return x & y;\n}\nstatic inline uint8_t or8(uint8_t x, uint8_t y)\n{\n    return x | y;\n}\nstatic inline uint16_t or16(uint16_t x, uint16_t y)\n{\n    return x | y;\n}\nstatic inline uint32_t or32(uint32_t x, uint32_t y)\n{\n    return x | y;\n}\nstatic inline uint64_t or64(uint64_t x, uint64_t y)\n{\n    return x | y;\n}\nstatic inline uint8_t xor8(uint8_t x, uint8_t y)\n{\n    return x ^ y;\n}\nstatic inline uint16_t xor16(uint16_t x, uint16_t y)\n{\n    return x ^ y;\n}\nstatic inline uint32_t xor32(uint32_t x, uint32_t y)\n{\n    return x ^ y;\n}\nstatic inline uint64_t xor64(uint64_t x, uint64_t y)\n{\n    return x ^ y;\n}\nstatic inline char ult8(uint8_t x, uint8_t y)\n{\n    return x < y;\n}\nstatic inline char ult16(uint16_t x, uint16_t y)\n{\n    return x < y;\n}\nstatic inline char ult32(uint32_t x, uint32_t y)\n{\n    return x < y;\n}\nstatic inline char ult64(uint64_t x, uint64_t y)\n{\n    return x < y;\n}\nstatic inline char ule8(uint8_t x, uint8_t y)\n{\n    return x <= y;\n}\nstatic inline char ule16(uint16_t x, uint16_t y)\n{\n    return x <= y;\n}\nstatic inline char ule32(uint32_t x, uint32_t y)\n{\n    return x <= y;\n}\nstatic inline char ule64(uint64_t x, uint64_t y)\n{\n    return x <= y;\n}\nstatic inline char slt8(int8_t x, int8_t y)\n{\n    return x < y;\n}\nstatic inline char slt16(int16_t x, int16_t y)\n{\n    return x < y;\n}\nstatic inline char slt32(int32_t x, int32_t y)\n{\n    return x < y;\n}\nstatic inline char slt64(int64_t x, int64_t y)\n{\n    return x < y;\n}\nstatic inline char sle8(int8_t x, int8_t y)\n{\n    return x <= y;\n}\nstatic inline char sle16(int16_t x, int16_t y)\n{\n    return x <= y;\n}\nstatic inline char sle32(int32_t x, int32_t y)\n{\n    return x <= y;\n}\nstatic inline char sle64(int64_t x, int64_",
            "t y)\n{\n    return x <= y;\n}\nstatic inline int8_t pow8(int8_t x, int8_t y)\n{\n    int8_t res = 1, rem = y;\n    \n    while (rem != 0) {\n        if (rem & 1)\n            res *= x;\n        rem >>= 1;\n        x *= x;\n    }\n    return res;\n}\nstatic inline int16_t pow16(int16_t x, int16_t y)\n{\n    int16_t res = 1, rem = y;\n    \n    while (rem != 0) {\n        if (rem & 1)\n            res *= x;\n        rem >>= 1;\n        x *= x;\n    }\n    return res;\n}\nstatic inline int32_t pow32(int32_t x, int32_t y)\n{\n    int32_t res = 1, rem = y;\n    \n    while (rem != 0) {\n        if (rem & 1)\n            res *= x;\n        rem >>= 1;\n        x *= x;\n    }\n    return res;\n}\nstatic inline int64_t pow64(int64_t x, int64_t y)\n{\n    int64_t res = 1, rem = y;\n    \n    while (rem != 0) {\n        if (rem & 1)\n            res *= x;\n        rem >>= 1;\n        x *= x;\n    }\n    return res;\n}\nstatic inline int8_t sext_i8_i8(int8_t x)\n{\n    return x;\n}\nstatic inline int16_t sext_i8_i16(int8_t x)\n{\n    return x;\n}\nstatic inline int32_t sext_i8_i32(int8_t x)\n{\n    return x;\n}\nstatic inline int64_t sext_i8_i64(int8_t x)\n{\n    return x;\n}\nstatic inline int8_t sext_i16_i8(int16_t x)\n{\n    return x;\n}\nstatic inline int16_t sext_i16_i16(int16_t x)\n{\n    return x;\n}\nstatic inline int32_t sext_i16_i32(int16_t x)\n{\n    return x;\n}\nstatic inline int64_t sext_i16_i64(int16_t x)\n{\n    return x;\n}\nstatic inline int8_t sext_i32_i8(int32_t x)\n{\n    return x;\n}\nstatic inline int16_t sext_i32_i16(int32_t x)\n{\n    return x;\n}\nstatic inline int32_t sext_i32_i32(int32_t x)\n{\n    return x;\n}\nstatic inline int64_t sext_i32_i64(int32_t x)\n{\n    return x;\n}\nstatic inline int8_t sext_i64_i8(int64_t x)\n{\n    return x;\n}\nstatic inline int16_t sext_i64_i16(int64_t x)\n{\n    return x;\n}\nstatic inline int32_t sext_i64_i32(int64_t x)\n{\n    return x;\n}\nstatic inline int64_t sext_i64_i64(int64_t x)\n{\n    return x;\n}\nstatic inline uint8_t zext_i8_i8(uint8_t x)\n{\n    return x;\n}\nstatic inline uint16_t zext_i8_i16(uint8_t x)\n{\n    return ",
            "x;\n}\nstatic inline uint32_t zext_i8_i32(uint8_t x)\n{\n    return x;\n}\nstatic inline uint64_t zext_i8_i64(uint8_t x)\n{\n    return x;\n}\nstatic inline uint8_t zext_i16_i8(uint16_t x)\n{\n    return x;\n}\nstatic inline uint16_t zext_i16_i16(uint16_t x)\n{\n    return x;\n}\nstatic inline uint32_t zext_i16_i32(uint16_t x)\n{\n    return x;\n}\nstatic inline uint64_t zext_i16_i64(uint16_t x)\n{\n    return x;\n}\nstatic inline uint8_t zext_i32_i8(uint32_t x)\n{\n    return x;\n}\nstatic inline uint16_t zext_i32_i16(uint32_t x)\n{\n    return x;\n}\nstatic inline uint32_t zext_i32_i32(uint32_t x)\n{\n    return x;\n}\nstatic inline uint64_t zext_i32_i64(uint32_t x)\n{\n    return x;\n}\nstatic inline uint8_t zext_i64_i8(uint64_t x)\n{\n    return x;\n}\nstatic inline uint16_t zext_i64_i16(uint64_t x)\n{\n    return x;\n}\nstatic inline uint32_t zext_i64_i32(uint64_t x)\n{\n    return x;\n}\nstatic inline uint64_t zext_i64_i64(uint64_t x)\n{\n    return x;\n}\nstatic inline float fdiv32(float x, float y)\n{\n    return x / y;\n}\nstatic inline float fadd32(float x, float y)\n{\n    return x + y;\n}\nstatic inline float fsub32(float x, float y)\n{\n    return x - y;\n}\nstatic inline float fmul32(float x, float y)\n{\n    return x * y;\n}\nstatic inline float fmin32(float x, float y)\n{\n    return x < y ? x : y;\n}\nstatic inline float fmax32(float x, float y)\n{\n    return x < y ? y : x;\n}\nstatic inline float fpow32(float x, float y)\n{\n    return pow(x, y);\n}\nstatic inline char cmplt32(float x, float y)\n{\n    return x < y;\n}\nstatic inline char cmple32(float x, float y)\n{\n    return x <= y;\n}\nstatic inline float sitofp_i8_f32(int8_t x)\n{\n    return x;\n}\nstatic inline float sitofp_i16_f32(int16_t x)\n{\n    return x;\n}\nstatic inline float sitofp_i32_f32(int32_t x)\n{\n    return x;\n}\nstatic inline float sitofp_i64_f32(int64_t x)\n{\n    return x;\n}\nstatic inline float uitofp_i8_f32(uint8_t x)\n{\n    return x;\n}\nstatic inline float uitofp_i16_f32(uint16_t x)\n{\n    return x;\n}\nstatic inline float uitofp_i32_f32(uint32_t x)\n{\n    return x;\n}\nstatic inl",
            "ine float uitofp_i64_f32(uint64_t x)\n{\n    return x;\n}\nstatic inline int8_t fptosi_f32_i8(float x)\n{\n    return x;\n}\nstatic inline int16_t fptosi_f32_i16(float x)\n{\n    return x;\n}\nstatic inline int32_t fptosi_f32_i32(float x)\n{\n    return x;\n}\nstatic inline int64_t fptosi_f32_i64(float x)\n{\n    return x;\n}\nstatic inline uint8_t fptoui_f32_i8(float x)\n{\n    return x;\n}\nstatic inline uint16_t fptoui_f32_i16(float x)\n{\n    return x;\n}\nstatic inline uint32_t fptoui_f32_i32(float x)\n{\n    return x;\n}\nstatic inline uint64_t fptoui_f32_i64(float x)\n{\n    return x;\n}\nstatic inline float futrts_log32(float x)\n{\n    return log(x);\n}\nstatic inline float futrts_log2_32(float x)\n{\n    return log2(x);\n}\nstatic inline float futrts_log10_32(float x)\n{\n    return log10(x);\n}\nstatic inline float futrts_sqrt32(float x)\n{\n    return sqrt(x);\n}\nstatic inline float futrts_exp32(float x)\n{\n    return exp(x);\n}\nstatic inline float futrts_cos32(float x)\n{\n    return cos(x);\n}\nstatic inline float futrts_sin32(float x)\n{\n    return sin(x);\n}\nstatic inline float futrts_tan32(float x)\n{\n    return tan(x);\n}\nstatic inline float futrts_acos32(float x)\n{\n    return acos(x);\n}\nstatic inline float futrts_asin32(float x)\n{\n    return asin(x);\n}\nstatic inline float futrts_atan32(float x)\n{\n    return atan(x);\n}\nstatic inline float futrts_atan2_32(float x, float y)\n{\n    return atan2(x, y);\n}\nstatic inline float futrts_round32(float x)\n{\n    return rint(x);\n}\nstatic inline char futrts_isnan32(float x)\n{\n    return isnan(x);\n}\nstatic inline char futrts_isinf32(float x)\n{\n    return isinf(x);\n}\nstatic inline int32_t futrts_to_bits32(float x)\n{\n    union {\n        float f;\n        int32_t t;\n    } p;\n    \n    p.f = x;\n    return p.t;\n}\nstatic inline float futrts_from_bits32(int32_t x)\n{\n    union {\n        int32_t f;\n        float t;\n    } p;\n    \n    p.f = x;\n    return p.t;\n}\nstatic inline double fdiv64(double x, double y)\n{\n    return x / y;\n}\nstatic inline double fadd64(double x, double y)\n{\n    retu",
            "rn x + y;\n}\nstatic inline double fsub64(double x, double y)\n{\n    return x - y;\n}\nstatic inline double fmul64(double x, double y)\n{\n    return x * y;\n}\nstatic inline double fmin64(double x, double y)\n{\n    return x < y ? x : y;\n}\nstatic inline double fmax64(double x, double y)\n{\n    return x < y ? y : x;\n}\nstatic inline double fpow64(double x, double y)\n{\n    return pow(x, y);\n}\nstatic inline char cmplt64(double x, double y)\n{\n    return x < y;\n}\nstatic inline char cmple64(double x, double y)\n{\n    return x <= y;\n}\nstatic inline double sitofp_i8_f64(int8_t x)\n{\n    return x;\n}\nstatic inline double sitofp_i16_f64(int16_t x)\n{\n    return x;\n}\nstatic inline double sitofp_i32_f64(int32_t x)\n{\n    return x;\n}\nstatic inline double sitofp_i64_f64(int64_t x)\n{\n    return x;\n}\nstatic inline double uitofp_i8_f64(uint8_t x)\n{\n    return x;\n}\nstatic inline double uitofp_i16_f64(uint16_t x)\n{\n    return x;\n}\nstatic inline double uitofp_i32_f64(uint32_t x)\n{\n    return x;\n}\nstatic inline double uitofp_i64_f64(uint64_t x)\n{\n    return x;\n}\nstatic inline int8_t fptosi_f64_i8(double x)\n{\n    return x;\n}\nstatic inline int16_t fptosi_f64_i16(double x)\n{\n    return x;\n}\nstatic inline int32_t fptosi_f64_i32(double x)\n{\n    return x;\n}\nstatic inline int64_t fptosi_f64_i64(double x)\n{\n    return x;\n}\nstatic inline uint8_t fptoui_f64_i8(double x)\n{\n    return x;\n}\nstatic inline uint16_t fptoui_f64_i16(double x)\n{\n    return x;\n}\nstatic inline uint32_t fptoui_f64_i32(double x)\n{\n    return x;\n}\nstatic inline uint64_t fptoui_f64_i64(double x)\n{\n    return x;\n}\nstatic inline double futrts_log64(double x)\n{\n    return log(x);\n}\nstatic inline double futrts_log2_64(double x)\n{\n    return log2(x);\n}\nstatic inline double futrts_log10_64(double x)\n{\n    return log10(x);\n}\nstatic inline double futrts_sqrt64(double x)\n{\n    return sqrt(x);\n}\nstatic inline double futrts_exp64(double x)\n{\n    return exp(x);\n}\nstatic inline double futrts_cos64(double x)\n{\n    return cos(x);\n}\nstatic inline double futrts",
            "_sin64(double x)\n{\n    return sin(x);\n}\nstatic inline double futrts_tan64(double x)\n{\n    return tan(x);\n}\nstatic inline double futrts_acos64(double x)\n{\n    return acos(x);\n}\nstatic inline double futrts_asin64(double x)\n{\n    return asin(x);\n}\nstatic inline double futrts_atan64(double x)\n{\n    return atan(x);\n}\nstatic inline double futrts_atan2_64(double x, double y)\n{\n    return atan2(x, y);\n}\nstatic inline double futrts_round64(double x)\n{\n    return rint(x);\n}\nstatic inline char futrts_isnan64(double x)\n{\n    return isnan(x);\n}\nstatic inline char futrts_isinf64(double x)\n{\n    return isinf(x);\n}\nstatic inline int64_t futrts_to_bits64(double x)\n{\n    union {\n        double f;\n        int64_t t;\n    } p;\n    \n    p.f = x;\n    return p.t;\n}\nstatic inline double futrts_from_bits64(int64_t x)\n{\n    union {\n        int64_t f;\n        double t;\n    } p;\n    \n    p.f = x;\n    return p.t;\n}\nstatic inline float fpconv_f32_f32(float x)\n{\n    return x;\n}\nstatic inline double fpconv_f32_f64(float x)\n{\n    return x;\n}\nstatic inline float fpconv_f64_f32(double x)\n{\n    return x;\n}\nstatic inline double fpconv_f64_f64(double x)\n{\n    return x;\n}\n#define group_sizze_4952 (group_size_4951)\n#define max_num_groups_4954 (max_num_groups_4953)\n#define group_sizze_5013 (group_size_5012)\n#define max_num_groups_5015 (max_num_groups_5014)\n#define group_sizze_5085 (group_size_5084)\n#define max_num_groups_5087 (max_num_groups_5086)\n#define group_sizze_5244 (group_size_5243)\n#define max_num_groups_5246 (max_num_groups_5245)\n__kernel void chunked_reduce_kernel_4968(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         int32_t sizze_4805,\n                                         int32_t num_threads_4960,\n                                         int32_t per_thread_elements_4963,\n                                         __global unsigned char *col_mem_5425,\n                                         __global unsigned char *mem_543",
            "1)\n{\n    __local volatile char *restrict mem_5428 = mem_aligned_0;\n    int32_t wave_sizze_5481;\n    int32_t group_sizze_5482;\n    bool thread_active_5483;\n    int32_t global_tid_4968;\n    int32_t local_tid_4969;\n    int32_t group_id_4970;\n    \n    global_tid_4968 = get_global_id(0);\n    local_tid_4969 = get_local_id(0);\n    group_sizze_5482 = get_local_size(0);\n    wave_sizze_5481 = LOCKSTEP_WIDTH;\n    group_id_4970 = get_group_id(0);\n    thread_active_5483 = 1;\n    \n    int32_t chunk_sizze_4975 = smin32(per_thread_elements_4963,\n                                      squot32(sizze_4805 - global_tid_4968 +\n                                              num_threads_4960 - 1,\n                                              num_threads_4960));\n    double res_4978;\n    \n    if (thread_active_5483) {\n        double acc_4981 = 0.0;\n        \n        for (int32_t i_4980 = 0; i_4980 < chunk_sizze_4975; i_4980++) {\n            int32_t j_t_s_5406 = num_threads_4960 * i_4980;\n            int32_t j_p_i_t_s_5407 = global_tid_4968 + j_t_s_5406;\n            double x_4983 = *(__global double *) &col_mem_5425[j_p_i_t_s_5407 *\n                                                               8];\n            double res_4986 = acc_4981 + x_4983;\n            double acc_tmp_5484 = res_4986;\n            \n            acc_4981 = acc_tmp_5484;\n        }\n        res_4978 = acc_4981;\n    }\n    \n    double final_result_4989;\n    \n    for (int32_t comb_iter_5485 = 0; comb_iter_5485 < squot32(group_sizze_4952 +\n                                                              group_sizze_4952 -\n                                                              1,\n                                                              group_sizze_4952);\n         comb_iter_5485++) {\n        int32_t combine_id_4973;\n        int32_t flat_comb_id_5486 = comb_iter_5485 * group_sizze_4952 +\n                local_tid_4969;\n        \n        combine_id_4973 = flat_comb_id_5486;\n        if (slt32(combine_id_4973, group_sizze_4952) &&",
            " 1) {\n            *(__local double *) &mem_5428[combine_id_4973 * 8] = res_4978;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5488;\n    int32_t skip_waves_5487;\n    int32_t my_index_4990;\n    int32_t other_index_4991;\n    double x_4992;\n    double x_4993;\n    \n    my_index_4990 = local_tid_4969;\n    offset_5488 = 0;\n    other_index_4991 = local_tid_4969 + offset_5488;\n    if (slt32(local_tid_4969, group_sizze_4952)) {\n        x_4992 = *(__local double *) &mem_5428[(local_tid_4969 + offset_5488) *\n                                               8];\n    }\n    offset_5488 = 1;\n    other_index_4991 = local_tid_4969 + offset_5488;\n    while (slt32(offset_5488, wave_sizze_5481)) {\n        if (slt32(other_index_4991, group_sizze_4952) && ((local_tid_4969 -\n                                                           squot32(local_tid_4969,\n                                                                   wave_sizze_5481) *\n                                                           wave_sizze_5481) &\n                                                          (2 * offset_5488 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_4993 = *(volatile __local\n                           double *) &mem_5428[(local_tid_4969 + offset_5488) *\n                                               8];\n            }\n            \n            double res_4994;\n            \n            if (thread_active_5483) {\n                res_4994 = x_4992 + x_4993;\n            }\n            x_4992 = res_4994;\n            *(volatile __local double *) &mem_5428[local_tid_4969 * 8] = x_4992;\n        }\n        offset_5488 *= 2;\n        other_index_4991 = local_tid_4969 + offset_5488;\n    }\n    skip_waves_5487 = 1;\n    while (slt32(skip_waves_5487, squot32(group_sizze_4952 + wave_sizze_5481 -\n                                          1, wave_sizze_5481))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        of",
            "fset_5488 = skip_waves_5487 * wave_sizze_5481;\n        other_index_4991 = local_tid_4969 + offset_5488;\n        if (slt32(other_index_4991, group_sizze_4952) && ((local_tid_4969 -\n                                                           squot32(local_tid_4969,\n                                                                   wave_sizze_5481) *\n                                                           wave_sizze_5481) ==\n                                                          0 &&\n                                                          (squot32(local_tid_4969,\n                                                                   wave_sizze_5481) &\n                                                           (2 *\n                                                            skip_waves_5487 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_4993 = *(__local double *) &mem_5428[(local_tid_4969 +\n                                                        offset_5488) * 8];\n            }\n            \n            double res_4994;\n            \n            if (thread_active_5483) {\n                res_4994 = x_4992 + x_4993;\n            }\n            x_4992 = res_4994;\n            *(__local double *) &mem_5428[local_tid_4969 * 8] = x_4992;\n        }\n        skip_waves_5487 *= 2;\n    }\n    final_result_4989 = x_4992;\n    if (local_tid_4969 == 0) {\n        *(__global double *) &mem_5431[group_id_4970 * 8] = final_result_4989;\n    }\n}\n__kernel void chunked_reduce_kernel_5029(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         int32_t sizze_4812,\n                                         int32_t num_threads_5021,\n                                         int32_t per_thread_elements_5024,\n                                         __global unsigned char *col_mem_5425,\n                                         __global unsigned c",
            "har *mem_5431)\n{\n    __local volatile char *restrict mem_5428 = mem_aligned_0;\n    int32_t wave_sizze_5499;\n    int32_t group_sizze_5500;\n    bool thread_active_5501;\n    int32_t global_tid_5029;\n    int32_t local_tid_5030;\n    int32_t group_id_5031;\n    \n    global_tid_5029 = get_global_id(0);\n    local_tid_5030 = get_local_id(0);\n    group_sizze_5500 = get_local_size(0);\n    wave_sizze_5499 = LOCKSTEP_WIDTH;\n    group_id_5031 = get_group_id(0);\n    thread_active_5501 = 1;\n    \n    int32_t chunk_sizze_5036 = smin32(per_thread_elements_5024,\n                                      squot32(sizze_4812 - global_tid_5029 +\n                                              num_threads_5021 - 1,\n                                              num_threads_5021));\n    double res_5039;\n    \n    if (thread_active_5501) {\n        double acc_5042 = 0.0;\n        \n        for (int32_t i_5041 = 0; i_5041 < chunk_sizze_5036; i_5041++) {\n            int32_t j_t_s_5406 = num_threads_5021 * i_5041;\n            int32_t j_p_i_t_s_5407 = global_tid_5029 + j_t_s_5406;\n            double x_5044 = *(__global double *) &col_mem_5425[j_p_i_t_s_5407 *\n                                                               8];\n            double res_5047 = acc_5042 + x_5044;\n            double acc_tmp_5502 = res_5047;\n            \n            acc_5042 = acc_tmp_5502;\n        }\n        res_5039 = acc_5042;\n    }\n    \n    double final_result_5050;\n    \n    for (int32_t comb_iter_5503 = 0; comb_iter_5503 < squot32(group_sizze_5013 +\n                                                              group_sizze_5013 -\n                                                              1,\n                                                              group_sizze_5013);\n         comb_iter_5503++) {\n        int32_t combine_id_5034;\n        int32_t flat_comb_id_5504 = comb_iter_5503 * group_sizze_5013 +\n                local_tid_5030;\n        \n        combine_id_5034 = flat_comb_id_5504;\n        if (slt32(combine_id_5034, group_si",
            "zze_5013) && 1) {\n            *(__local double *) &mem_5428[combine_id_5034 * 8] = res_5039;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5506;\n    int32_t skip_waves_5505;\n    int32_t my_index_5051;\n    int32_t other_index_5052;\n    double x_5053;\n    double x_5054;\n    \n    my_index_5051 = local_tid_5030;\n    offset_5506 = 0;\n    other_index_5052 = local_tid_5030 + offset_5506;\n    if (slt32(local_tid_5030, group_sizze_5013)) {\n        x_5053 = *(__local double *) &mem_5428[(local_tid_5030 + offset_5506) *\n                                               8];\n    }\n    offset_5506 = 1;\n    other_index_5052 = local_tid_5030 + offset_5506;\n    while (slt32(offset_5506, wave_sizze_5499)) {\n        if (slt32(other_index_5052, group_sizze_5013) && ((local_tid_5030 -\n                                                           squot32(local_tid_5030,\n                                                                   wave_sizze_5499) *\n                                                           wave_sizze_5499) &\n                                                          (2 * offset_5506 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5054 = *(volatile __local\n                           double *) &mem_5428[(local_tid_5030 + offset_5506) *\n                                               8];\n            }\n            \n            double res_5055;\n            \n            if (thread_active_5501) {\n                res_5055 = x_5053 + x_5054;\n            }\n            x_5053 = res_5055;\n            *(volatile __local double *) &mem_5428[local_tid_5030 * 8] = x_5053;\n        }\n        offset_5506 *= 2;\n        other_index_5052 = local_tid_5030 + offset_5506;\n    }\n    skip_waves_5505 = 1;\n    while (slt32(skip_waves_5505, squot32(group_sizze_5013 + wave_sizze_5499 -\n                                          1, wave_sizze_5499))) {\n        barrier(CLK_LOCAL_MEM_FENCE)",
            ";\n        offset_5506 = skip_waves_5505 * wave_sizze_5499;\n        other_index_5052 = local_tid_5030 + offset_5506;\n        if (slt32(other_index_5052, group_sizze_5013) && ((local_tid_5030 -\n                                                           squot32(local_tid_5030,\n                                                                   wave_sizze_5499) *\n                                                           wave_sizze_5499) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5030,\n                                                                   wave_sizze_5499) &\n                                                           (2 *\n                                                            skip_waves_5505 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5054 = *(__local double *) &mem_5428[(local_tid_5030 +\n                                                        offset_5506) * 8];\n            }\n            \n            double res_5055;\n            \n            if (thread_active_5501) {\n                res_5055 = x_5053 + x_5054;\n            }\n            x_5053 = res_5055;\n            *(__local double *) &mem_5428[local_tid_5030 * 8] = x_5053;\n        }\n        skip_waves_5505 *= 2;\n    }\n    final_result_5050 = x_5053;\n    if (local_tid_5030 == 0) {\n        *(__global double *) &mem_5431[group_id_5031 * 8] = final_result_5050;\n    }\n}\n__kernel void chunked_reduce_kernel_5103(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         __local volatile\n                                         int64_t *mem_aligned_1,\n                                         __local volatile\n                                         int64_t *mem_aligned_2,\n                                         int32_t sizze_4821,\n                       ",
            "                  int32_t num_threads_5093,\n                                         int32_t per_thread_elements_5096,\n                                         int32_t per_chunk_5395, __global\n                                         unsigned char *mem_5435, __global\n                                         unsigned char *mem_5447, __global\n                                         unsigned char *mem_5450, __global\n                                         unsigned char *mem_5453)\n{\n    __local volatile char *restrict mem_5438 = mem_aligned_0;\n    __local volatile char *restrict mem_5441 = mem_aligned_1;\n    __local volatile char *restrict mem_5444 = mem_aligned_2;\n    int32_t wave_sizze_5518;\n    int32_t group_sizze_5519;\n    bool thread_active_5520;\n    int32_t global_tid_5103;\n    int32_t local_tid_5104;\n    int32_t group_id_5105;\n    \n    global_tid_5103 = get_global_id(0);\n    local_tid_5104 = get_local_id(0);\n    group_sizze_5519 = get_local_size(0);\n    wave_sizze_5518 = LOCKSTEP_WIDTH;\n    group_id_5105 = get_group_id(0);\n    thread_active_5520 = 1;\n    \n    int32_t chunk_sizze_5119;\n    int32_t starting_point_5521 = global_tid_5103 * per_thread_elements_5096;\n    int32_t remaining_elements_5522 = sizze_4821 - starting_point_5521;\n    \n    if (sle32(remaining_elements_5522, 0) || sle32(sizze_4821,\n                                                   starting_point_5521)) {\n        chunk_sizze_5119 = 0;\n    } else {\n        if (slt32(sizze_4821, (global_tid_5103 + 1) *\n                  per_thread_elements_5096)) {\n            chunk_sizze_5119 = sizze_4821 - global_tid_5103 *\n                per_thread_elements_5096;\n        } else {\n            chunk_sizze_5119 = per_thread_elements_5096;\n        }\n    }\n    \n    int32_t slice_offset_5120;\n    double res_5125;\n    bool cond_5126;\n    double res_5127;\n    double res_5128;\n    double res_5129;\n    \n    if (thread_active_5520) {\n        slice_offset_5120 = per_thread_elements_5096 * global_tid_5103;\n        res_512",
            "5 = sitofp_i32_f64(chunk_sizze_5119);\n        cond_5126 = res_5125 == 0.0;\n        if (cond_5126) {\n            res_5127 = 0.0;\n        } else {\n            res_5127 = res_5125;\n        }\n        if (cond_5126) {\n            res_5128 = 0.0;\n            res_5129 = 0.0;\n        } else {\n            double res_5130;\n            double res_5144;\n            double res_5145;\n            double res_5161;\n            double x_5133 = 0.0;\n            \n            for (int32_t chunk_offset_5132 = 0; chunk_offset_5132 <\n                 chunk_sizze_5119; chunk_offset_5132++) {\n                int32_t j_p_i_t_s_5409 = slice_offset_5120 + chunk_offset_5132;\n                int32_t new_index_5410 = squot32(j_p_i_t_s_5409,\n                                                 per_chunk_5395);\n                int32_t binop_y_5412 = per_chunk_5395 * new_index_5410;\n                int32_t new_index_5413 = j_p_i_t_s_5409 - binop_y_5412;\n                double x_5140 = *(__global double *) &mem_5435[(new_index_5413 *\n                                                                num_threads_5093 +\n                                                                new_index_5410) *\n                                                               8];\n                double res_5143 = x_5133 + x_5140;\n                double x_tmp_5523 = res_5143;\n                \n                x_5133 = x_tmp_5523;\n            }\n            res_5130 = x_5133;\n            res_5144 = res_5130 / res_5125;\n            \n            double x_5148 = 0.0;\n            \n            for (int32_t chunk_offset_5147 = 0; chunk_offset_5147 <\n                 chunk_sizze_5119; chunk_offset_5147++) {\n                int32_t j_p_i_t_s_5419 = slice_offset_5120 + chunk_offset_5147;\n                int32_t new_index_5420 = squot32(j_p_i_t_s_5419,\n                                                 per_chunk_5395);\n                int32_t binop_y_5422 = per_chunk_5395 * new_index_5420;\n                int32_t new_index_5423 = j_p_i_t_s",
            "_5419 - binop_y_5422;\n                double x_5155 = *(__global double *) &mem_5435[(new_index_5423 *\n                                                                num_threads_5093 +\n                                                                new_index_5420) *\n                                                               8];\n                double x_5157 = x_5155 - res_5144;\n                double res_5158 = x_5157 * x_5157;\n                double res_5160 = x_5148 + res_5158;\n                double x_tmp_5524 = res_5160;\n                \n                x_5148 = x_tmp_5524;\n            }\n            res_5145 = x_5148;\n            res_5161 = res_5145 / res_5125;\n            res_5128 = res_5144;\n            res_5129 = res_5161;\n        }\n    }\n    \n    double final_result_5168;\n    double final_result_5169;\n    double final_result_5170;\n    \n    for (int32_t comb_iter_5525 = 0; comb_iter_5525 < squot32(group_sizze_5085 +\n                                                              group_sizze_5085 -\n                                                              1,\n                                                              group_sizze_5085);\n         comb_iter_5525++) {\n        int32_t combine_id_5113;\n        int32_t flat_comb_id_5526 = comb_iter_5525 * group_sizze_5085 +\n                local_tid_5104;\n        \n        combine_id_5113 = flat_comb_id_5526;\n        if (slt32(combine_id_5113, group_sizze_5085) && 1) {\n            *(__local double *) &mem_5438[combine_id_5113 * 8] = res_5128;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    for (int32_t comb_iter_5527 = 0; comb_iter_5527 < squot32(group_sizze_5085 +\n                                                              group_sizze_5085 -\n                                                              1,\n                                                              group_sizze_5085);\n         comb_iter_5527++) {\n        int32_t combine_id_5114;\n        int32_t flat_comb_id_5528 = comb_iter_5527 * gr",
            "oup_sizze_5085 +\n                local_tid_5104;\n        \n        combine_id_5114 = flat_comb_id_5528;\n        if (slt32(combine_id_5114, group_sizze_5085) && 1) {\n            *(__local double *) &mem_5441[combine_id_5114 * 8] = res_5127;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    for (int32_t comb_iter_5529 = 0; comb_iter_5529 < squot32(group_sizze_5085 +\n                                                              group_sizze_5085 -\n                                                              1,\n                                                              group_sizze_5085);\n         comb_iter_5529++) {\n        int32_t combine_id_5115;\n        int32_t flat_comb_id_5530 = comb_iter_5529 * group_sizze_5085 +\n                local_tid_5104;\n        \n        combine_id_5115 = flat_comb_id_5530;\n        if (slt32(combine_id_5115, group_sizze_5085) && 1) {\n            *(__local double *) &mem_5444[combine_id_5115 * 8] = res_5129;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5532;\n    int32_t skip_waves_5531;\n    int32_t my_index_5171;\n    int32_t other_index_5172;\n    double x_5173;\n    double x_5174;\n    double x_5175;\n    double x_5176;\n    double x_5177;\n    double x_5178;\n    \n    my_index_5171 = local_tid_5104;\n    offset_5532 = 0;\n    other_index_5172 = local_tid_5104 + offset_5532;\n    if (slt32(local_tid_5104, group_sizze_5085)) {\n        x_5173 = *(__local double *) &mem_5438[(local_tid_5104 + offset_5532) *\n                                               8];\n        x_5174 = *(__local double *) &mem_5441[(local_tid_5104 + offset_5532) *\n                                               8];\n        x_5175 = *(__local double *) &mem_5444[(local_tid_5104 + offset_5532) *\n                                               8];\n    }\n    offset_5532 = 1;\n    other_index_5172 = local_tid_5104 + offset_5532;\n    while (slt32(offset_5532, wave_sizze_5518)) {\n        if (slt32(other_index_5172, group_sizze_5085) && ((local_tid_5104 -\n  ",
            "                                                         squot32(local_tid_5104,\n                                                                   wave_sizze_5518) *\n                                                           wave_sizze_5518) &\n                                                          (2 * offset_5532 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5176 = *(volatile __local\n                           double *) &mem_5438[(local_tid_5104 + offset_5532) *\n                                               8];\n                x_5177 = *(volatile __local\n                           double *) &mem_5441[(local_tid_5104 + offset_5532) *\n                                               8];\n                x_5178 = *(volatile __local\n                           double *) &mem_5444[(local_tid_5104 + offset_5532) *\n                                               8];\n            }\n            \n            bool cond_5179;\n            double res_5180;\n            double res_5181;\n            double res_5182;\n            \n            if (thread_active_5520) {\n                cond_5179 = x_5174 == 0.0;\n                if (cond_5179) {\n                    res_5180 = x_5176;\n                    res_5181 = x_5177;\n                    res_5182 = x_5178;\n                } else {\n                    bool cond_5183;\n                    double res_5184;\n                    double res_5185;\n                    double res_5186;\n                    \n                    cond_5183 = x_5177 == 0.0;\n                    if (cond_5183) {\n                        res_5184 = x_5173;\n                        res_5185 = x_5174;\n                        res_5186 = x_5175;\n                    } else {\n                        double res_5187;\n                        double res_5188;\n                        double res_5189;\n                        double x_5190;\n                        double res_5191;\n          ",
            "              double y_5192;\n                        double res_5193;\n                        double y_5194;\n                        double res_5195;\n                        double res_5196;\n                        double x_5197;\n                        double x_5198;\n                        double x_5199;\n                        double x_5200;\n                        double y_5201;\n                        double res_5202;\n                        double y_5203;\n                        double res_5204;\n                        \n                        res_5187 = x_5174 + x_5177;\n                        res_5188 = x_5173 * x_5174;\n                        res_5189 = x_5176 * x_5177;\n                        x_5190 = res_5188 + res_5189;\n                        res_5191 = x_5190 / res_5187;\n                        y_5192 = x_5174 - 1.0;\n                        res_5193 = x_5175 * y_5192;\n                        y_5194 = x_5177 - 1.0;\n                        res_5195 = x_5178 * y_5194;\n                        res_5196 = x_5176 - x_5173;\n                        x_5197 = res_5193 + res_5195;\n                        x_5198 = res_5196 * res_5196;\n                        x_5199 = x_5174 * x_5198;\n                        x_5200 = x_5177 * x_5199;\n                        y_5201 = x_5200 / res_5187;\n                        res_5202 = x_5197 + y_5201;\n                        y_5203 = res_5187 - 1.0;\n                        res_5204 = res_5202 / y_5203;\n                        res_5184 = res_5191;\n                        res_5185 = res_5187;\n                        res_5186 = res_5204;\n                    }\n                    res_5180 = res_5184;\n                    res_5181 = res_5185;\n                    res_5182 = res_5186;\n                }\n            }\n            x_5173 = res_5180;\n            x_5174 = res_5181;\n            x_5175 = res_5182;\n            *(volatile __local double *) &mem_5438[local_tid_5104 * 8] = x_5173;\n            *(volatile __local double *) &mem_5441[lo",
            "cal_tid_5104 * 8] = x_5174;\n            *(volatile __local double *) &mem_5444[local_tid_5104 * 8] = x_5175;\n        }\n        offset_5532 *= 2;\n        other_index_5172 = local_tid_5104 + offset_5532;\n    }\n    skip_waves_5531 = 1;\n    while (slt32(skip_waves_5531, squot32(group_sizze_5085 + wave_sizze_5518 -\n                                          1, wave_sizze_5518))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5532 = skip_waves_5531 * wave_sizze_5518;\n        other_index_5172 = local_tid_5104 + offset_5532;\n        if (slt32(other_index_5172, group_sizze_5085) && ((local_tid_5104 -\n                                                           squot32(local_tid_5104,\n                                                                   wave_sizze_5518) *\n                                                           wave_sizze_5518) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5104,\n                                                                   wave_sizze_5518) &\n                                                           (2 *\n                                                            skip_waves_5531 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5176 = *(__local double *) &mem_5438[(local_tid_5104 +\n                                                        offset_5532) * 8];\n                x_5177 = *(__local double *) &mem_5441[(local_tid_5104 +\n                                                        offset_5532) * 8];\n                x_5178 = *(__local double *) &mem_5444[(local_tid_5104 +\n                                                        offset_5532) * 8];\n            }\n            \n            bool cond_5179;\n            double res_5180;\n            double res_5181;\n            double res_5182;\n            \n            if (thread_active_5520) {\n              ",
            "  cond_5179 = x_5174 == 0.0;\n                if (cond_5179) {\n                    res_5180 = x_5176;\n                    res_5181 = x_5177;\n                    res_5182 = x_5178;\n                } else {\n                    bool cond_5183;\n                    double res_5184;\n                    double res_5185;\n                    double res_5186;\n                    \n                    cond_5183 = x_5177 == 0.0;\n                    if (cond_5183) {\n                        res_5184 = x_5173;\n                        res_5185 = x_5174;\n                        res_5186 = x_5175;\n                    } else {\n                        double res_5187;\n                        double res_5188;\n                        double res_5189;\n                        double x_5190;\n                        double res_5191;\n                        double y_5192;\n                        double res_5193;\n                        double y_5194;\n                        double res_5195;\n                        double res_5196;\n                        double x_5197;\n                        double x_5198;\n                        double x_5199;\n                        double x_5200;\n                        double y_5201;\n                        double res_5202;\n                        double y_5203;\n                        double res_5204;\n                        \n                        res_5187 = x_5174 + x_5177;\n                        res_5188 = x_5173 * x_5174;\n                        res_5189 = x_5176 * x_5177;\n                        x_5190 = res_5188 + res_5189;\n                        res_5191 = x_5190 / res_5187;\n                        y_5192 = x_5174 - 1.0;\n                        res_5193 = x_5175 * y_5192;\n                        y_5194 = x_5177 - 1.0;\n                        res_5195 = x_5178 * y_5194;\n                        res_5196 = x_5176 - x_5173;\n                        x_5197 = res_5193 + res_5195;\n                        x_5198 = res_5196 * res_5196;\n                   ",
            "     x_5199 = x_5174 * x_5198;\n                        x_5200 = x_5177 * x_5199;\n                        y_5201 = x_5200 / res_5187;\n                        res_5202 = x_5197 + y_5201;\n                        y_5203 = res_5187 - 1.0;\n                        res_5204 = res_5202 / y_5203;\n                        res_5184 = res_5191;\n                        res_5185 = res_5187;\n                        res_5186 = res_5204;\n                    }\n                    res_5180 = res_5184;\n                    res_5181 = res_5185;\n                    res_5182 = res_5186;\n                }\n            }\n            x_5173 = res_5180;\n            x_5174 = res_5181;\n            x_5175 = res_5182;\n            *(__local double *) &mem_5438[local_tid_5104 * 8] = x_5173;\n            *(__local double *) &mem_5441[local_tid_5104 * 8] = x_5174;\n            *(__local double *) &mem_5444[local_tid_5104 * 8] = x_5175;\n        }\n        skip_waves_5531 *= 2;\n    }\n    final_result_5168 = x_5173;\n    final_result_5169 = x_5174;\n    final_result_5170 = x_5175;\n    if (local_tid_5104 == 0) {\n        *(__global double *) &mem_5447[group_id_5105 * 8] = final_result_5168;\n    }\n    if (local_tid_5104 == 0) {\n        *(__global double *) &mem_5450[group_id_5105 * 8] = final_result_5169;\n    }\n    if (local_tid_5104 == 0) {\n        *(__global double *) &mem_5453[group_id_5105 * 8] = final_result_5170;\n    }\n}\n__kernel void chunked_reduce_kernel_5262(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         __local volatile\n                                         int64_t *mem_aligned_1,\n                                         __local volatile\n                                         int64_t *mem_aligned_2,\n                                         int32_t sizze_4882,\n                                         int32_t num_threads_5252,\n                                         int32_t per_thread_elements_5255,\n                           ",
            "              int32_t per_chunk_5395, __global\n                                         unsigned char *mem_5435, __global\n                                         unsigned char *mem_5447, __global\n                                         unsigned char *mem_5450, __global\n                                         unsigned char *mem_5453)\n{\n    __local volatile char *restrict mem_5438 = mem_aligned_0;\n    __local volatile char *restrict mem_5441 = mem_aligned_1;\n    __local volatile char *restrict mem_5444 = mem_aligned_2;\n    int32_t wave_sizze_5548;\n    int32_t group_sizze_5549;\n    bool thread_active_5550;\n    int32_t global_tid_5262;\n    int32_t local_tid_5263;\n    int32_t group_id_5264;\n    \n    global_tid_5262 = get_global_id(0);\n    local_tid_5263 = get_local_id(0);\n    group_sizze_5549 = get_local_size(0);\n    wave_sizze_5548 = LOCKSTEP_WIDTH;\n    group_id_5264 = get_group_id(0);\n    thread_active_5550 = 1;\n    \n    int32_t chunk_sizze_5278;\n    int32_t starting_point_5551 = global_tid_5262 * per_thread_elements_5255;\n    int32_t remaining_elements_5552 = sizze_4882 - starting_point_5551;\n    \n    if (sle32(remaining_elements_5552, 0) || sle32(sizze_4882,\n                                                   starting_point_5551)) {\n        chunk_sizze_5278 = 0;\n    } else {\n        if (slt32(sizze_4882, (global_tid_5262 + 1) *\n                  per_thread_elements_5255)) {\n            chunk_sizze_5278 = sizze_4882 - global_tid_5262 *\n                per_thread_elements_5255;\n        } else {\n            chunk_sizze_5278 = per_thread_elements_5255;\n        }\n    }\n    \n    int32_t slice_offset_5279;\n    double res_5284;\n    bool cond_5285;\n    double res_5286;\n    double res_5287;\n    double res_5288;\n    \n    if (thread_active_5550) {\n        slice_offset_5279 = per_thread_elements_5255 * global_tid_5262;\n        res_5284 = sitofp_i32_f64(chunk_sizze_5278);\n        cond_5285 = res_5284 == 0.0;\n        if (cond_5285) {\n            res_5286 = 0.0;\n        } else {\n ",
            "           res_5286 = res_5284;\n        }\n        if (cond_5285) {\n            res_5287 = 0.0;\n            res_5288 = 0.0;\n        } else {\n            double res_5289;\n            double res_5303;\n            double res_5304;\n            double res_5320;\n            double x_5292 = 0.0;\n            \n            for (int32_t chunk_offset_5291 = 0; chunk_offset_5291 <\n                 chunk_sizze_5278; chunk_offset_5291++) {\n                int32_t j_p_i_t_s_5409 = slice_offset_5279 + chunk_offset_5291;\n                int32_t new_index_5410 = squot32(j_p_i_t_s_5409,\n                                                 per_chunk_5395);\n                int32_t binop_y_5412 = per_chunk_5395 * new_index_5410;\n                int32_t new_index_5413 = j_p_i_t_s_5409 - binop_y_5412;\n                double x_5299 = *(__global double *) &mem_5435[(new_index_5413 *\n                                                                num_threads_5252 +\n                                                                new_index_5410) *\n                                                               8];\n                double res_5302 = x_5292 + x_5299;\n                double x_tmp_5553 = res_5302;\n                \n                x_5292 = x_tmp_5553;\n            }\n            res_5289 = x_5292;\n            res_5303 = res_5289 / res_5284;\n            \n            double x_5307 = 0.0;\n            \n            for (int32_t chunk_offset_5306 = 0; chunk_offset_5306 <\n                 chunk_sizze_5278; chunk_offset_5306++) {\n                int32_t j_p_i_t_s_5419 = slice_offset_5279 + chunk_offset_5306;\n                int32_t new_index_5420 = squot32(j_p_i_t_s_5419,\n                                                 per_chunk_5395);\n                int32_t binop_y_5422 = per_chunk_5395 * new_index_5420;\n                int32_t new_index_5423 = j_p_i_t_s_5419 - binop_y_5422;\n                double x_5314 = *(__global double *) &mem_5435[(new_index_5423 *\n                                           ",
            "                     num_threads_5252 +\n                                                                new_index_5420) *\n                                                               8];\n                double x_5316 = x_5314 - res_5303;\n                double res_5317 = x_5316 * x_5316;\n                double res_5319 = x_5307 + res_5317;\n                double x_tmp_5554 = res_5319;\n                \n                x_5307 = x_tmp_5554;\n            }\n            res_5304 = x_5307;\n            res_5320 = res_5304 / res_5284;\n            res_5287 = res_5303;\n            res_5288 = res_5320;\n        }\n    }\n    \n    double final_result_5327;\n    double final_result_5328;\n    double final_result_5329;\n    \n    for (int32_t comb_iter_5555 = 0; comb_iter_5555 < squot32(group_sizze_5244 +\n                                                              group_sizze_5244 -\n                                                              1,\n                                                              group_sizze_5244);\n         comb_iter_5555++) {\n        int32_t combine_id_5272;\n        int32_t flat_comb_id_5556 = comb_iter_5555 * group_sizze_5244 +\n                local_tid_5263;\n        \n        combine_id_5272 = flat_comb_id_5556;\n        if (slt32(combine_id_5272, group_sizze_5244) && 1) {\n            *(__local double *) &mem_5438[combine_id_5272 * 8] = res_5287;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    for (int32_t comb_iter_5557 = 0; comb_iter_5557 < squot32(group_sizze_5244 +\n                                                              group_sizze_5244 -\n                                                              1,\n                                                              group_sizze_5244);\n         comb_iter_5557++) {\n        int32_t combine_id_5273;\n        int32_t flat_comb_id_5558 = comb_iter_5557 * group_sizze_5244 +\n                local_tid_5263;\n        \n        combine_id_5273 = flat_comb_id_5558;\n        if (slt32(combine_id_5273, group_si",
            "zze_5244) && 1) {\n            *(__local double *) &mem_5441[combine_id_5273 * 8] = res_5286;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    for (int32_t comb_iter_5559 = 0; comb_iter_5559 < squot32(group_sizze_5244 +\n                                                              group_sizze_5244 -\n                                                              1,\n                                                              group_sizze_5244);\n         comb_iter_5559++) {\n        int32_t combine_id_5274;\n        int32_t flat_comb_id_5560 = comb_iter_5559 * group_sizze_5244 +\n                local_tid_5263;\n        \n        combine_id_5274 = flat_comb_id_5560;\n        if (slt32(combine_id_5274, group_sizze_5244) && 1) {\n            *(__local double *) &mem_5444[combine_id_5274 * 8] = res_5288;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5562;\n    int32_t skip_waves_5561;\n    int32_t my_index_5330;\n    int32_t other_index_5331;\n    double x_5332;\n    double x_5333;\n    double x_5334;\n    double x_5335;\n    double x_5336;\n    double x_5337;\n    \n    my_index_5330 = local_tid_5263;\n    offset_5562 = 0;\n    other_index_5331 = local_tid_5263 + offset_5562;\n    if (slt32(local_tid_5263, group_sizze_5244)) {\n        x_5332 = *(__local double *) &mem_5438[(local_tid_5263 + offset_5562) *\n                                               8];\n        x_5333 = *(__local double *) &mem_5441[(local_tid_5263 + offset_5562) *\n                                               8];\n        x_5334 = *(__local double *) &mem_5444[(local_tid_5263 + offset_5562) *\n                                               8];\n    }\n    offset_5562 = 1;\n    other_index_5331 = local_tid_5263 + offset_5562;\n    while (slt32(offset_5562, wave_sizze_5548)) {\n        if (slt32(other_index_5331, group_sizze_5244) && ((local_tid_5263 -\n                                                           squot32(local_tid_5263,\n                                                                 ",
            "  wave_sizze_5548) *\n                                                           wave_sizze_5548) &\n                                                          (2 * offset_5562 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5335 = *(volatile __local\n                           double *) &mem_5438[(local_tid_5263 + offset_5562) *\n                                               8];\n                x_5336 = *(volatile __local\n                           double *) &mem_5441[(local_tid_5263 + offset_5562) *\n                                               8];\n                x_5337 = *(volatile __local\n                           double *) &mem_5444[(local_tid_5263 + offset_5562) *\n                                               8];\n            }\n            \n            bool cond_5338;\n            double res_5339;\n            double res_5340;\n            double res_5341;\n            \n            if (thread_active_5550) {\n                cond_5338 = x_5333 == 0.0;\n                if (cond_5338) {\n                    res_5339 = x_5335;\n                    res_5340 = x_5336;\n                    res_5341 = x_5337;\n                } else {\n                    bool cond_5342;\n                    double res_5343;\n                    double res_5344;\n                    double res_5345;\n                    \n                    cond_5342 = x_5336 == 0.0;\n                    if (cond_5342) {\n                        res_5343 = x_5332;\n                        res_5344 = x_5333;\n                        res_5345 = x_5334;\n                    } else {\n                        double res_5346;\n                        double res_5347;\n                        double res_5348;\n                        double x_5349;\n                        double res_5350;\n                        double y_5351;\n                        double res_5352;\n                        double y_5353;\n                        double res_53",
            "54;\n                        double res_5355;\n                        double x_5356;\n                        double x_5357;\n                        double x_5358;\n                        double x_5359;\n                        double y_5360;\n                        double res_5361;\n                        double y_5362;\n                        double res_5363;\n                        \n                        res_5346 = x_5333 + x_5336;\n                        res_5347 = x_5332 * x_5333;\n                        res_5348 = x_5335 * x_5336;\n                        x_5349 = res_5347 + res_5348;\n                        res_5350 = x_5349 / res_5346;\n                        y_5351 = x_5333 - 1.0;\n                        res_5352 = x_5334 * y_5351;\n                        y_5353 = x_5336 - 1.0;\n                        res_5354 = x_5337 * y_5353;\n                        res_5355 = x_5335 - x_5332;\n                        x_5356 = res_5352 + res_5354;\n                        x_5357 = res_5355 * res_5355;\n                        x_5358 = x_5333 * x_5357;\n                        x_5359 = x_5336 * x_5358;\n                        y_5360 = x_5359 / res_5346;\n                        res_5361 = x_5356 + y_5360;\n                        y_5362 = res_5346 - 1.0;\n                        res_5363 = res_5361 / y_5362;\n                        res_5343 = res_5350;\n                        res_5344 = res_5346;\n                        res_5345 = res_5363;\n                    }\n                    res_5339 = res_5343;\n                    res_5340 = res_5344;\n                    res_5341 = res_5345;\n                }\n            }\n            x_5332 = res_5339;\n            x_5333 = res_5340;\n            x_5334 = res_5341;\n            *(volatile __local double *) &mem_5438[local_tid_5263 * 8] = x_5332;\n            *(volatile __local double *) &mem_5441[local_tid_5263 * 8] = x_5333;\n            *(volatile __local double *) &mem_5444[local_tid_5263 * 8] = x_5334;\n        }\n        offset_5562 *= 2;\n ",
            "       other_index_5331 = local_tid_5263 + offset_5562;\n    }\n    skip_waves_5561 = 1;\n    while (slt32(skip_waves_5561, squot32(group_sizze_5244 + wave_sizze_5548 -\n                                          1, wave_sizze_5548))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5562 = skip_waves_5561 * wave_sizze_5548;\n        other_index_5331 = local_tid_5263 + offset_5562;\n        if (slt32(other_index_5331, group_sizze_5244) && ((local_tid_5263 -\n                                                           squot32(local_tid_5263,\n                                                                   wave_sizze_5548) *\n                                                           wave_sizze_5548) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5263,\n                                                                   wave_sizze_5548) &\n                                                           (2 *\n                                                            skip_waves_5561 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5335 = *(__local double *) &mem_5438[(local_tid_5263 +\n                                                        offset_5562) * 8];\n                x_5336 = *(__local double *) &mem_5441[(local_tid_5263 +\n                                                        offset_5562) * 8];\n                x_5337 = *(__local double *) &mem_5444[(local_tid_5263 +\n                                                        offset_5562) * 8];\n            }\n            \n            bool cond_5338;\n            double res_5339;\n            double res_5340;\n            double res_5341;\n            \n            if (thread_active_5550) {\n                cond_5338 = x_5333 == 0.0;\n                if (cond_5338) {\n                    res_5339 = x_5335;\n                    res_5340 = x_5336;\n      ",
            "              res_5341 = x_5337;\n                } else {\n                    bool cond_5342;\n                    double res_5343;\n                    double res_5344;\n                    double res_5345;\n                    \n                    cond_5342 = x_5336 == 0.0;\n                    if (cond_5342) {\n                        res_5343 = x_5332;\n                        res_5344 = x_5333;\n                        res_5345 = x_5334;\n                    } else {\n                        double res_5346;\n                        double res_5347;\n                        double res_5348;\n                        double x_5349;\n                        double res_5350;\n                        double y_5351;\n                        double res_5352;\n                        double y_5353;\n                        double res_5354;\n                        double res_5355;\n                        double x_5356;\n                        double x_5357;\n                        double x_5358;\n                        double x_5359;\n                        double y_5360;\n                        double res_5361;\n                        double y_5362;\n                        double res_5363;\n                        \n                        res_5346 = x_5333 + x_5336;\n                        res_5347 = x_5332 * x_5333;\n                        res_5348 = x_5335 * x_5336;\n                        x_5349 = res_5347 + res_5348;\n                        res_5350 = x_5349 / res_5346;\n                        y_5351 = x_5333 - 1.0;\n                        res_5352 = x_5334 * y_5351;\n                        y_5353 = x_5336 - 1.0;\n                        res_5354 = x_5337 * y_5353;\n                        res_5355 = x_5335 - x_5332;\n                        x_5356 = res_5352 + res_5354;\n                        x_5357 = res_5355 * res_5355;\n                        x_5358 = x_5333 * x_5357;\n                        x_5359 = x_5336 * x_5358;\n                        y_5360 = x_5359 / res_5346;\n             ",
            "           res_5361 = x_5356 + y_5360;\n                        y_5362 = res_5346 - 1.0;\n                        res_5363 = res_5361 / y_5362;\n                        res_5343 = res_5350;\n                        res_5344 = res_5346;\n                        res_5345 = res_5363;\n                    }\n                    res_5339 = res_5343;\n                    res_5340 = res_5344;\n                    res_5341 = res_5345;\n                }\n            }\n            x_5332 = res_5339;\n            x_5333 = res_5340;\n            x_5334 = res_5341;\n            *(__local double *) &mem_5438[local_tid_5263 * 8] = x_5332;\n            *(__local double *) &mem_5441[local_tid_5263 * 8] = x_5333;\n            *(__local double *) &mem_5444[local_tid_5263 * 8] = x_5334;\n        }\n        skip_waves_5561 *= 2;\n    }\n    final_result_5327 = x_5332;\n    final_result_5328 = x_5333;\n    final_result_5329 = x_5334;\n    if (local_tid_5263 == 0) {\n        *(__global double *) &mem_5447[group_id_5264 * 8] = final_result_5327;\n    }\n    if (local_tid_5263 == 0) {\n        *(__global double *) &mem_5450[group_id_5264 * 8] = final_result_5328;\n    }\n    if (local_tid_5263 == 0) {\n        *(__global double *) &mem_5453[group_id_5264 * 8] = final_result_5329;\n    }\n}\n__kernel void fut_kernel_map_transpose_f64(__global double *odata,\n                                           uint odata_offset, __global\n                                           double *idata, uint idata_offset,\n                                           uint width, uint height,\n                                           uint input_size, uint output_size,\n                                           __local double *block)\n{\n    uint x_index;\n    uint y_index;\n    uint our_array_offset;\n    \n    // Adjust the input and output arrays with the basic offset.\n    odata += odata_offset / sizeof(double);\n    idata += idata_offset / sizeof(double);\n    // Adjust the input and output arrays for the third dimension.\n    our_array_offset = get_g",
            "lobal_id(2) * width * height;\n    odata += our_array_offset;\n    idata += our_array_offset;\n    // read the matrix tile into shared memory\n    x_index = get_global_id(0);\n    y_index = get_global_id(1);\n    \n    uint index_in = y_index * width + x_index;\n    \n    if ((x_index < width && y_index < height) && index_in < input_size)\n        block[get_local_id(1) * (FUT_BLOCK_DIM + 1) + get_local_id(0)] =\n            idata[index_in];\n    barrier(CLK_LOCAL_MEM_FENCE);\n    // Scatter the transposed matrix tile to global memory.\n    x_index = get_group_id(1) * FUT_BLOCK_DIM + get_local_id(0);\n    y_index = get_group_id(0) * FUT_BLOCK_DIM + get_local_id(1);\n    \n    uint index_out = y_index * height + x_index;\n    \n    if ((x_index < height && y_index < width) && index_out < output_size)\n        odata[index_out] = block[get_local_id(0) * (FUT_BLOCK_DIM + 1) +\n                                 get_local_id(1)];\n}\n__kernel void fut_kernel_map_transpose_lowheight_f64(__global double *odata,\n                                                     uint odata_offset, __global\n                                                     double *idata,\n                                                     uint idata_offset,\n                                                     uint width, uint height,\n                                                     uint input_size,\n                                                     uint output_size,\n                                                     uint mulx, __local\n                                                     double *block)\n{\n    uint x_index;\n    uint y_index;\n    uint our_array_offset;\n    \n    // Adjust the input and output arrays with the basic offset.\n    odata += odata_offset / sizeof(double);\n    idata += idata_offset / sizeof(double);\n    // Adjust the input and output arrays for the third dimension.\n    our_array_offset = get_global_id(2) * width * height;\n    odata += our_array_offset;\n    idata += our_array_offset;\n    // read the ",
            "matrix tile into shared memory\n    x_index = get_group_id(0) * FUT_BLOCK_DIM * mulx + get_local_id(0) +\n        get_local_id(1) % mulx * FUT_BLOCK_DIM;\n    y_index = get_group_id(1) * FUT_BLOCK_DIM + get_local_id(1) / mulx;\n    \n    uint index_in = y_index * width + x_index;\n    \n    if ((x_index < width && y_index < height) && index_in < input_size)\n        block[get_local_id(1) * (FUT_BLOCK_DIM + 1) + get_local_id(0)] =\n            idata[index_in];\n    barrier(CLK_LOCAL_MEM_FENCE);\n    // Scatter the transposed matrix tile to global memory.\n    x_index = get_group_id(1) * FUT_BLOCK_DIM + get_local_id(0) / mulx;\n    y_index = get_group_id(0) * FUT_BLOCK_DIM * mulx + get_local_id(1) +\n        get_local_id(0) % mulx * FUT_BLOCK_DIM;\n    \n    uint index_out = y_index * height + x_index;\n    \n    if ((x_index < height && y_index < width) && index_out < output_size)\n        odata[index_out] = block[get_local_id(0) * (FUT_BLOCK_DIM + 1) +\n                                 get_local_id(1)];\n}\n__kernel void fut_kernel_map_transpose_lowwidth_f64(__global double *odata,\n                                                    uint odata_offset, __global\n                                                    double *idata,\n                                                    uint idata_offset,\n                                                    uint width, uint height,\n                                                    uint input_size,\n                                                    uint output_size, uint muly,\n                                                    __local double *block)\n{\n    uint x_index;\n    uint y_index;\n    uint our_array_offset;\n    \n    // Adjust the input and output arrays with the basic offset.\n    odata += odata_offset / sizeof(double);\n    idata += idata_offset / sizeof(double);\n    // Adjust the input and output arrays for the third dimension.\n    our_array_offset = get_global_id(2) * width * height;\n    odata += our_array_offset;\n    idata += our_array_o",
            "ffset;\n    // read the matrix tile into shared memory\n    x_index = get_group_id(0) * FUT_BLOCK_DIM + get_local_id(0) / muly;\n    y_index = get_group_id(1) * FUT_BLOCK_DIM * muly + get_local_id(1) +\n        get_local_id(0) % muly * FUT_BLOCK_DIM;\n    \n    uint index_in = y_index * width + x_index;\n    \n    if ((x_index < width && y_index < height) && index_in < input_size)\n        block[get_local_id(1) * (FUT_BLOCK_DIM + 1) + get_local_id(0)] =\n            idata[index_in];\n    barrier(CLK_LOCAL_MEM_FENCE);\n    // Scatter the transposed matrix tile to global memory.\n    x_index = get_group_id(1) * FUT_BLOCK_DIM * muly + get_local_id(0) +\n        get_local_id(1) % muly * FUT_BLOCK_DIM;\n    y_index = get_group_id(0) * FUT_BLOCK_DIM + get_local_id(1) / muly;\n    \n    uint index_out = y_index * height + x_index;\n    \n    if ((x_index < height && y_index < width) && index_out < output_size)\n        odata[index_out] = block[get_local_id(0) * (FUT_BLOCK_DIM + 1) +\n                                 get_local_id(1)];\n}\n__kernel void fut_kernel_map_transpose_small_f64(__global double *odata,\n                                                 uint odata_offset, __global\n                                                 double *idata,\n                                                 uint idata_offset,\n                                                 uint num_arrays, uint width,\n                                                 uint height, uint input_size,\n                                                 uint output_size)\n{\n    uint our_array_offset = get_global_id(0) / (height * width) * (height *\n                                                                   width);\n    uint x_index = get_global_id(0) % (height * width) / height;\n    uint y_index = get_global_id(0) % height;\n    \n    // Adjust the input and output arrays with the basic offset.\n    odata += odata_offset / sizeof(double);\n    idata += idata_offset / sizeof(double);\n    // Adjust the input and output arrays.\n    o",
            "data += our_array_offset;\n    idata += our_array_offset;\n    \n    uint index_in = y_index * width + x_index;\n    uint index_out = x_index * height + y_index;\n    \n    if (get_global_id(0) < input_size)\n        odata[index_out] = idata[index_in];\n}\n__kernel void reduce_kernel_4996(__local volatile int64_t *mem_aligned_0,\n                                 int32_t num_groups_4959, __global\n                                 unsigned char *mem_5431, __global\n                                 unsigned char *mem_5437)\n{\n    __local volatile char *restrict mem_5434 = mem_aligned_0;\n    int32_t wave_sizze_5490;\n    int32_t group_sizze_5491;\n    bool thread_active_5492;\n    int32_t global_tid_4996;\n    int32_t local_tid_4997;\n    int32_t group_id_4998;\n    \n    global_tid_4996 = get_global_id(0);\n    local_tid_4997 = get_local_id(0);\n    group_sizze_5491 = get_local_size(0);\n    wave_sizze_5490 = LOCKSTEP_WIDTH;\n    group_id_4998 = get_group_id(0);\n    thread_active_5492 = 1;\n    \n    bool in_bounds_4999;\n    double x_5384;\n    \n    if (thread_active_5492) {\n        in_bounds_4999 = slt32(local_tid_4997, num_groups_4959);\n        if (in_bounds_4999) {\n            double x_5000 = *(__global double *) &mem_5431[global_tid_4996 * 8];\n            \n            x_5384 = x_5000;\n        } else {\n            x_5384 = 0.0;\n        }\n    }\n    \n    double final_result_5004;\n    \n    for (int32_t comb_iter_5493 = 0; comb_iter_5493 <\n         squot32(max_num_groups_4954 + max_num_groups_4954 - 1,\n                 max_num_groups_4954); comb_iter_5493++) {\n        int32_t combine_id_5003;\n        int32_t flat_comb_id_5494 = comb_iter_5493 * max_num_groups_4954 +\n                local_tid_4997;\n        \n        combine_id_5003 = flat_comb_id_5494;\n        if (slt32(combine_id_5003, max_num_groups_4954) && 1) {\n            *(__local double *) &mem_5434[combine_id_5003 * 8] = x_5384;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5496;\n    int32_t skip_waves_5495;\n    ",
            "double x_4808;\n    double x_4809;\n    int32_t my_index_4966;\n    int32_t other_index_4967;\n    \n    my_index_4966 = local_tid_4997;\n    offset_5496 = 0;\n    other_index_4967 = local_tid_4997 + offset_5496;\n    if (slt32(local_tid_4997, max_num_groups_4954)) {\n        x_4808 = *(__local double *) &mem_5434[(local_tid_4997 + offset_5496) *\n                                               8];\n    }\n    offset_5496 = 1;\n    other_index_4967 = local_tid_4997 + offset_5496;\n    while (slt32(offset_5496, wave_sizze_5490)) {\n        if (slt32(other_index_4967, max_num_groups_4954) && ((local_tid_4997 -\n                                                              squot32(local_tid_4997,\n                                                                      wave_sizze_5490) *\n                                                              wave_sizze_5490) &\n                                                             (2 * offset_5496 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4809 = *(volatile __local\n                           double *) &mem_5434[(local_tid_4997 + offset_5496) *\n                                               8];\n            }\n            \n            double res_4810;\n            \n            if (thread_active_5492) {\n                res_4810 = x_4808 + x_4809;\n            }\n            x_4808 = res_4810;\n            *(volatile __local double *) &mem_5434[local_tid_4997 * 8] = x_4808;\n        }\n        offset_5496 *= 2;\n        other_index_4967 = local_tid_4997 + offset_5496;\n    }\n    skip_waves_5495 = 1;\n    while (slt32(skip_waves_5495, squot32(max_num_groups_4954 +\n                                          wave_sizze_5490 - 1,\n                                          wave_sizze_5490))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5496 = skip_waves_5495 * wave_sizze_5490;\n        other_index_4967 = local_tid_4997 + offset_5496;\n        if (slt32(other_",
            "index_4967, max_num_groups_4954) && ((local_tid_4997 -\n                                                              squot32(local_tid_4997,\n                                                                      wave_sizze_5490) *\n                                                              wave_sizze_5490) ==\n                                                             0 &&\n                                                             (squot32(local_tid_4997,\n                                                                      wave_sizze_5490) &\n                                                              (2 *\n                                                               skip_waves_5495 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4809 = *(__local double *) &mem_5434[(local_tid_4997 +\n                                                        offset_5496) * 8];\n            }\n            \n            double res_4810;\n            \n            if (thread_active_5492) {\n                res_4810 = x_4808 + x_4809;\n            }\n            x_4808 = res_4810;\n            *(__local double *) &mem_5434[local_tid_4997 * 8] = x_4808;\n        }\n        skip_waves_5495 *= 2;\n    }\n    final_result_5004 = x_4808;\n    if (local_tid_4997 == 0) {\n        *(__global double *) &mem_5437[group_id_4998 * 8] = final_result_5004;\n    }\n}\n__kernel void reduce_kernel_5057(__local volatile int64_t *mem_aligned_0,\n                                 int32_t num_groups_5020, __global\n                                 unsigned char *mem_5431, __global\n                                 unsigned char *mem_5437)\n{\n    __local volatile char *restrict mem_5434 = mem_aligned_0;\n    int32_t wave_sizze_5508;\n    int32_t group_sizze_5509;\n    bool thread_active_5510;\n    int32_t global_tid_5057;\n    int32_t local_tid_5058;\n    int32_t group_id_5059;\n    \n    global_tid_5057 = get_global_id(0);\n    local_tid_50",
            "58 = get_local_id(0);\n    group_sizze_5509 = get_local_size(0);\n    wave_sizze_5508 = LOCKSTEP_WIDTH;\n    group_id_5059 = get_group_id(0);\n    thread_active_5510 = 1;\n    \n    bool in_bounds_5060;\n    double x_5384;\n    \n    if (thread_active_5510) {\n        in_bounds_5060 = slt32(local_tid_5058, num_groups_5020);\n        if (in_bounds_5060) {\n            double x_5061 = *(__global double *) &mem_5431[global_tid_5057 * 8];\n            \n            x_5384 = x_5061;\n        } else {\n            x_5384 = 0.0;\n        }\n    }\n    \n    double final_result_5065;\n    \n    for (int32_t comb_iter_5511 = 0; comb_iter_5511 <\n         squot32(max_num_groups_5015 + max_num_groups_5015 - 1,\n                 max_num_groups_5015); comb_iter_5511++) {\n        int32_t combine_id_5064;\n        int32_t flat_comb_id_5512 = comb_iter_5511 * max_num_groups_5015 +\n                local_tid_5058;\n        \n        combine_id_5064 = flat_comb_id_5512;\n        if (slt32(combine_id_5064, max_num_groups_5015) && 1) {\n            *(__local double *) &mem_5434[combine_id_5064 * 8] = x_5384;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5514;\n    int32_t skip_waves_5513;\n    double x_4815;\n    double x_4816;\n    int32_t my_index_5027;\n    int32_t other_index_5028;\n    \n    my_index_5027 = local_tid_5058;\n    offset_5514 = 0;\n    other_index_5028 = local_tid_5058 + offset_5514;\n    if (slt32(local_tid_5058, max_num_groups_5015)) {\n        x_4815 = *(__local double *) &mem_5434[(local_tid_5058 + offset_5514) *\n                                               8];\n    }\n    offset_5514 = 1;\n    other_index_5028 = local_tid_5058 + offset_5514;\n    while (slt32(offset_5514, wave_sizze_5508)) {\n        if (slt32(other_index_5028, max_num_groups_5015) && ((local_tid_5058 -\n                                                              squot32(local_tid_5058,\n                                                                      wave_sizze_5508) *\n                                    ",
            "                          wave_sizze_5508) &\n                                                             (2 * offset_5514 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4816 = *(volatile __local\n                           double *) &mem_5434[(local_tid_5058 + offset_5514) *\n                                               8];\n            }\n            \n            double res_4817;\n            \n            if (thread_active_5510) {\n                res_4817 = x_4815 + x_4816;\n            }\n            x_4815 = res_4817;\n            *(volatile __local double *) &mem_5434[local_tid_5058 * 8] = x_4815;\n        }\n        offset_5514 *= 2;\n        other_index_5028 = local_tid_5058 + offset_5514;\n    }\n    skip_waves_5513 = 1;\n    while (slt32(skip_waves_5513, squot32(max_num_groups_5015 +\n                                          wave_sizze_5508 - 1,\n                                          wave_sizze_5508))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5514 = skip_waves_5513 * wave_sizze_5508;\n        other_index_5028 = local_tid_5058 + offset_5514;\n        if (slt32(other_index_5028, max_num_groups_5015) && ((local_tid_5058 -\n                                                              squot32(local_tid_5058,\n                                                                      wave_sizze_5508) *\n                                                              wave_sizze_5508) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5058,\n                                                                      wave_sizze_5508) &\n                                                              (2 *\n                                                               skip_waves_5513 -\n                                                               1)) == 0)) {\n            // read array element\n",
            "            {\n                x_4816 = *(__local double *) &mem_5434[(local_tid_5058 +\n                                                        offset_5514) * 8];\n            }\n            \n            double res_4817;\n            \n            if (thread_active_5510) {\n                res_4817 = x_4815 + x_4816;\n            }\n            x_4815 = res_4817;\n            *(__local double *) &mem_5434[local_tid_5058 * 8] = x_4815;\n        }\n        skip_waves_5513 *= 2;\n    }\n    final_result_5065 = x_4815;\n    if (local_tid_5058 == 0) {\n        *(__global double *) &mem_5437[group_id_5059 * 8] = final_result_5065;\n    }\n}\n__kernel void reduce_kernel_5208(__local volatile int64_t *mem_aligned_0,\n                                 __local volatile int64_t *mem_aligned_1,\n                                 __local volatile int64_t *mem_aligned_2,\n                                 int32_t num_groups_5092, __global\n                                 unsigned char *mem_5447, __global\n                                 unsigned char *mem_5450, __global\n                                 unsigned char *mem_5453, __global\n                                 unsigned char *mem_5465, __global\n                                 unsigned char *mem_5468, __global\n                                 unsigned char *mem_5471)\n{\n    __local volatile char *restrict mem_5456 = mem_aligned_0;\n    __local volatile char *restrict mem_5459 = mem_aligned_1;\n    __local volatile char *restrict mem_5462 = mem_aligned_2;\n    int32_t wave_sizze_5536;\n    int32_t group_sizze_5537;\n    bool thread_active_5538;\n    int32_t global_tid_5208;\n    int32_t local_tid_5209;\n    int32_t group_id_5210;\n    \n    global_tid_5208 = get_global_id(0);\n    local_tid_5209 = get_local_id(0);\n    group_sizze_5537 = get_local_size(0);\n    wave_sizze_5536 = LOCKSTEP_WIDTH;\n    group_id_5210 = get_group_id(0);\n    thread_active_5538 = 1;\n    \n    bool in_bounds_5211;\n    double x_5384;\n    double x_5386;\n    double x_5388;\n    \n    if (thre",
            "ad_active_5538) {\n        in_bounds_5211 = slt32(local_tid_5209, num_groups_5092);\n        if (in_bounds_5211) {\n            double x_5212 = *(__global double *) &mem_5447[global_tid_5208 * 8];\n            \n            x_5384 = x_5212;\n        } else {\n            x_5384 = 0.0;\n        }\n        if (in_bounds_5211) {\n            double x_5214 = *(__global double *) &mem_5450[global_tid_5208 * 8];\n            \n            x_5386 = x_5214;\n        } else {\n            x_5386 = 0.0;\n        }\n        if (in_bounds_5211) {\n            double x_5216 = *(__global double *) &mem_5453[global_tid_5208 * 8];\n            \n            x_5388 = x_5216;\n        } else {\n            x_5388 = 0.0;\n        }\n    }\n    \n    double final_result_5222;\n    double final_result_5223;\n    double final_result_5224;\n    \n    for (int32_t comb_iter_5539 = 0; comb_iter_5539 <\n         squot32(max_num_groups_5087 + max_num_groups_5087 - 1,\n                 max_num_groups_5087); comb_iter_5539++) {\n        int32_t combine_id_5221;\n        int32_t flat_comb_id_5540 = comb_iter_5539 * max_num_groups_5087 +\n                local_tid_5209;\n        \n        combine_id_5221 = flat_comb_id_5540;\n        if (slt32(combine_id_5221, max_num_groups_5087) && 1) {\n            *(__local double *) &mem_5456[combine_id_5221 * 8] = x_5384;\n            *(__local double *) &mem_5459[combine_id_5221 * 8] = x_5386;\n            *(__local double *) &mem_5462[combine_id_5221 * 8] = x_5388;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5542;\n    int32_t skip_waves_5541;\n    double x_4826;\n    double x_4827;\n    double x_4828;\n    double x_4829;\n    double x_4830;\n    double x_4831;\n    int32_t my_index_5101;\n    int32_t other_index_5102;\n    \n    my_index_5101 = local_tid_5209;\n    offset_5542 = 0;\n    other_index_5102 = local_tid_5209 + offset_5542;\n    if (slt32(local_tid_5209, max_num_groups_5087)) {\n        x_4826 = *(__local double *) &mem_5456[(local_tid_5209 + offset_5542) *\n          ",
            "                                     8];\n        x_4827 = *(__local double *) &mem_5459[(local_tid_5209 + offset_5542) *\n                                               8];\n        x_4828 = *(__local double *) &mem_5462[(local_tid_5209 + offset_5542) *\n                                               8];\n    }\n    offset_5542 = 1;\n    other_index_5102 = local_tid_5209 + offset_5542;\n    while (slt32(offset_5542, wave_sizze_5536)) {\n        if (slt32(other_index_5102, max_num_groups_5087) && ((local_tid_5209 -\n                                                              squot32(local_tid_5209,\n                                                                      wave_sizze_5536) *\n                                                              wave_sizze_5536) &\n                                                             (2 * offset_5542 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4829 = *(volatile __local\n                           double *) &mem_5456[(local_tid_5209 + offset_5542) *\n                                               8];\n                x_4830 = *(volatile __local\n                           double *) &mem_5459[(local_tid_5209 + offset_5542) *\n                                               8];\n                x_4831 = *(volatile __local\n                           double *) &mem_5462[(local_tid_5209 + offset_5542) *\n                                               8];\n            }\n            \n            bool cond_4832;\n            double res_4833;\n            double res_4834;\n            double res_4835;\n            \n            if (thread_active_5538) {\n                cond_4832 = x_4827 == 0.0;\n                if (cond_4832) {\n                    res_4833 = x_4829;\n                    res_4834 = x_4830;\n                    res_4835 = x_4831;\n                } else {\n                    bool cond_4836;\n                    double res_4837;\n                    d",
            "ouble res_4838;\n                    double res_4839;\n                    \n                    cond_4836 = x_4830 == 0.0;\n                    if (cond_4836) {\n                        res_4837 = x_4826;\n                        res_4838 = x_4827;\n                        res_4839 = x_4828;\n                    } else {\n                        double res_4840;\n                        double res_4841;\n                        double res_4842;\n                        double x_4843;\n                        double res_4844;\n                        double y_4845;\n                        double res_4846;\n                        double y_4847;\n                        double res_4848;\n                        double res_4849;\n                        double x_4850;\n                        double x_4851;\n                        double x_4852;\n                        double x_4853;\n                        double y_4854;\n                        double res_4855;\n                        double y_4856;\n                        double res_4857;\n                        \n                        res_4840 = x_4827 + x_4830;\n                        res_4841 = x_4826 * x_4827;\n                        res_4842 = x_4829 * x_4830;\n                        x_4843 = res_4841 + res_4842;\n                        res_4844 = x_4843 / res_4840;\n                        y_4845 = x_4827 - 1.0;\n                        res_4846 = x_4828 * y_4845;\n                        y_4847 = x_4830 - 1.0;\n                        res_4848 = x_4831 * y_4847;\n                        res_4849 = x_4829 - x_4826;\n                        x_4850 = res_4846 + res_4848;\n                        x_4851 = res_4849 * res_4849;\n                        x_4852 = x_4827 * x_4851;\n                        x_4853 = x_4830 * x_4852;\n                        y_4854 = x_4853 / res_4840;\n                        res_4855 = x_4850 + y_4854;\n                        y_4856 = res_4840 - 1.0;\n                        res_4857 = res_4855 / y_4856;\n          ",
            "              res_4837 = res_4844;\n                        res_4838 = res_4840;\n                        res_4839 = res_4857;\n                    }\n                    res_4833 = res_4837;\n                    res_4834 = res_4838;\n                    res_4835 = res_4839;\n                }\n            }\n            x_4826 = res_4833;\n            x_4827 = res_4834;\n            x_4828 = res_4835;\n            *(volatile __local double *) &mem_5456[local_tid_5209 * 8] = x_4826;\n            *(volatile __local double *) &mem_5459[local_tid_5209 * 8] = x_4827;\n            *(volatile __local double *) &mem_5462[local_tid_5209 * 8] = x_4828;\n        }\n        offset_5542 *= 2;\n        other_index_5102 = local_tid_5209 + offset_5542;\n    }\n    skip_waves_5541 = 1;\n    while (slt32(skip_waves_5541, squot32(max_num_groups_5087 +\n                                          wave_sizze_5536 - 1,\n                                          wave_sizze_5536))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5542 = skip_waves_5541 * wave_sizze_5536;\n        other_index_5102 = local_tid_5209 + offset_5542;\n        if (slt32(other_index_5102, max_num_groups_5087) && ((local_tid_5209 -\n                                                              squot32(local_tid_5209,\n                                                                      wave_sizze_5536) *\n                                                              wave_sizze_5536) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5209,\n                                                                      wave_sizze_5536) &\n                                                              (2 *\n                                                               skip_waves_5541 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4829 = *(__local double *) &m",
            "em_5456[(local_tid_5209 +\n                                                        offset_5542) * 8];\n                x_4830 = *(__local double *) &mem_5459[(local_tid_5209 +\n                                                        offset_5542) * 8];\n                x_4831 = *(__local double *) &mem_5462[(local_tid_5209 +\n                                                        offset_5542) * 8];\n            }\n            \n            bool cond_4832;\n            double res_4833;\n            double res_4834;\n            double res_4835;\n            \n            if (thread_active_5538) {\n                cond_4832 = x_4827 == 0.0;\n                if (cond_4832) {\n                    res_4833 = x_4829;\n                    res_4834 = x_4830;\n                    res_4835 = x_4831;\n                } else {\n                    bool cond_4836;\n                    double res_4837;\n                    double res_4838;\n                    double res_4839;\n                    \n                    cond_4836 = x_4830 == 0.0;\n                    if (cond_4836) {\n                        res_4837 = x_4826;\n                        res_4838 = x_4827;\n                        res_4839 = x_4828;\n                    } else {\n                        double res_4840;\n                        double res_4841;\n                        double res_4842;\n                        double x_4843;\n                        double res_4844;\n                        double y_4845;\n                        double res_4846;\n                        double y_4847;\n                        double res_4848;\n                        double res_4849;\n                        double x_4850;\n                        double x_4851;\n                        double x_4852;\n                        double x_4853;\n                        double y_4854;\n                        double res_4855;\n                        double y_4856;\n                        double res_4857;\n                        \n                        res_4840 = x_",
            "4827 + x_4830;\n                        res_4841 = x_4826 * x_4827;\n                        res_4842 = x_4829 * x_4830;\n                        x_4843 = res_4841 + res_4842;\n                        res_4844 = x_4843 / res_4840;\n                        y_4845 = x_4827 - 1.0;\n                        res_4846 = x_4828 * y_4845;\n                        y_4847 = x_4830 - 1.0;\n                        res_4848 = x_4831 * y_4847;\n                        res_4849 = x_4829 - x_4826;\n                        x_4850 = res_4846 + res_4848;\n                        x_4851 = res_4849 * res_4849;\n                        x_4852 = x_4827 * x_4851;\n                        x_4853 = x_4830 * x_4852;\n                        y_4854 = x_4853 / res_4840;\n                        res_4855 = x_4850 + y_4854;\n                        y_4856 = res_4840 - 1.0;\n                        res_4857 = res_4855 / y_4856;\n                        res_4837 = res_4844;\n                        res_4838 = res_4840;\n                        res_4839 = res_4857;\n                    }\n                    res_4833 = res_4837;\n                    res_4834 = res_4838;\n                    res_4835 = res_4839;\n                }\n            }\n            x_4826 = res_4833;\n            x_4827 = res_4834;\n            x_4828 = res_4835;\n            *(__local double *) &mem_5456[local_tid_5209 * 8] = x_4826;\n            *(__local double *) &mem_5459[local_tid_5209 * 8] = x_4827;\n            *(__local double *) &mem_5462[local_tid_5209 * 8] = x_4828;\n        }\n        skip_waves_5541 *= 2;\n    }\n    final_result_5222 = x_4826;\n    final_result_5223 = x_4827;\n    final_result_5224 = x_4828;\n    if (local_tid_5209 == 0) {\n        *(__global double *) &mem_5465[group_id_5210 * 8] = final_result_5222;\n    }\n    if (local_tid_5209 == 0) {\n        *(__global double *) &mem_5468[group_id_5210 * 8] = final_result_5223;\n    }\n    if (local_tid_5209 == 0) {\n        *(__global double *) &mem_5471[group_id_5210 * 8] = final_result_5224;\n   ",
            " }\n}\n__kernel void reduce_kernel_5367(__local volatile int64_t *mem_aligned_0,\n                                 __local volatile int64_t *mem_aligned_1,\n                                 __local volatile int64_t *mem_aligned_2,\n                                 int32_t num_groups_5251, __global\n                                 unsigned char *mem_5447, __global\n                                 unsigned char *mem_5450, __global\n                                 unsigned char *mem_5453, __global\n                                 unsigned char *mem_5465, __global\n                                 unsigned char *mem_5468, __global\n                                 unsigned char *mem_5471)\n{\n    __local volatile char *restrict mem_5456 = mem_aligned_0;\n    __local volatile char *restrict mem_5459 = mem_aligned_1;\n    __local volatile char *restrict mem_5462 = mem_aligned_2;\n    int32_t wave_sizze_5566;\n    int32_t group_sizze_5567;\n    bool thread_active_5568;\n    int32_t global_tid_5367;\n    int32_t local_tid_5368;\n    int32_t group_id_5369;\n    \n    global_tid_5367 = get_global_id(0);\n    local_tid_5368 = get_local_id(0);\n    group_sizze_5567 = get_local_size(0);\n    wave_sizze_5566 = LOCKSTEP_WIDTH;\n    group_id_5369 = get_group_id(0);\n    thread_active_5568 = 1;\n    \n    bool in_bounds_5370;\n    double x_5384;\n    double x_5386;\n    double x_5388;\n    \n    if (thread_active_5568) {\n        in_bounds_5370 = slt32(local_tid_5368, num_groups_5251);\n        if (in_bounds_5370) {\n            double x_5371 = *(__global double *) &mem_5447[global_tid_5367 * 8];\n            \n            x_5384 = x_5371;\n        } else {\n            x_5384 = 0.0;\n        }\n        if (in_bounds_5370) {\n            double x_5373 = *(__global double *) &mem_5450[global_tid_5367 * 8];\n            \n            x_5386 = x_5373;\n        } else {\n            x_5386 = 0.0;\n        }\n        if (in_bounds_5370) {\n            double x_5375 = *(__global double *) &mem_5453[global_tid_5367 * 8];\n            \n  ",
            "          x_5388 = x_5375;\n        } else {\n            x_5388 = 0.0;\n        }\n    }\n    \n    double final_result_5381;\n    double final_result_5382;\n    double final_result_5383;\n    \n    for (int32_t comb_iter_5569 = 0; comb_iter_5569 <\n         squot32(max_num_groups_5246 + max_num_groups_5246 - 1,\n                 max_num_groups_5246); comb_iter_5569++) {\n        int32_t combine_id_5380;\n        int32_t flat_comb_id_5570 = comb_iter_5569 * max_num_groups_5246 +\n                local_tid_5368;\n        \n        combine_id_5380 = flat_comb_id_5570;\n        if (slt32(combine_id_5380, max_num_groups_5246) && 1) {\n            *(__local double *) &mem_5456[combine_id_5380 * 8] = x_5384;\n            *(__local double *) &mem_5459[combine_id_5380 * 8] = x_5386;\n            *(__local double *) &mem_5462[combine_id_5380 * 8] = x_5388;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5572;\n    int32_t skip_waves_5571;\n    double x_4887;\n    double x_4888;\n    double x_4889;\n    double x_4890;\n    double x_4891;\n    double x_4892;\n    int32_t my_index_5260;\n    int32_t other_index_5261;\n    \n    my_index_5260 = local_tid_5368;\n    offset_5572 = 0;\n    other_index_5261 = local_tid_5368 + offset_5572;\n    if (slt32(local_tid_5368, max_num_groups_5246)) {\n        x_4887 = *(__local double *) &mem_5456[(local_tid_5368 + offset_5572) *\n                                               8];\n        x_4888 = *(__local double *) &mem_5459[(local_tid_5368 + offset_5572) *\n                                               8];\n        x_4889 = *(__local double *) &mem_5462[(local_tid_5368 + offset_5572) *\n                                               8];\n    }\n    offset_5572 = 1;\n    other_index_5261 = local_tid_5368 + offset_5572;\n    while (slt32(offset_5572, wave_sizze_5566)) {\n        if (slt32(other_index_5261, max_num_groups_5246) && ((local_tid_5368 -\n                                                              squot32(local_tid_5368,\n                       ",
            "                                               wave_sizze_5566) *\n                                                              wave_sizze_5566) &\n                                                             (2 * offset_5572 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4890 = *(volatile __local\n                           double *) &mem_5456[(local_tid_5368 + offset_5572) *\n                                               8];\n                x_4891 = *(volatile __local\n                           double *) &mem_5459[(local_tid_5368 + offset_5572) *\n                                               8];\n                x_4892 = *(volatile __local\n                           double *) &mem_5462[(local_tid_5368 + offset_5572) *\n                                               8];\n            }\n            \n            bool cond_4893;\n            double res_4894;\n            double res_4895;\n            double res_4896;\n            \n            if (thread_active_5568) {\n                cond_4893 = x_4888 == 0.0;\n                if (cond_4893) {\n                    res_4894 = x_4890;\n                    res_4895 = x_4891;\n                    res_4896 = x_4892;\n                } else {\n                    bool cond_4897;\n                    double res_4898;\n                    double res_4899;\n                    double res_4900;\n                    \n                    cond_4897 = x_4891 == 0.0;\n                    if (cond_4897) {\n                        res_4898 = x_4887;\n                        res_4899 = x_4888;\n                        res_4900 = x_4889;\n                    } else {\n                        double res_4901;\n                        double res_4902;\n                        double res_4903;\n                        double x_4904;\n                        double res_4905;\n                        double y_4906;\n                        double res_4907;\n                      ",
            "  double y_4908;\n                        double res_4909;\n                        double res_4910;\n                        double x_4911;\n                        double x_4912;\n                        double x_4913;\n                        double x_4914;\n                        double y_4915;\n                        double res_4916;\n                        double y_4917;\n                        double res_4918;\n                        \n                        res_4901 = x_4888 + x_4891;\n                        res_4902 = x_4887 * x_4888;\n                        res_4903 = x_4890 * x_4891;\n                        x_4904 = res_4902 + res_4903;\n                        res_4905 = x_4904 / res_4901;\n                        y_4906 = x_4888 - 1.0;\n                        res_4907 = x_4889 * y_4906;\n                        y_4908 = x_4891 - 1.0;\n                        res_4909 = x_4892 * y_4908;\n                        res_4910 = x_4890 - x_4887;\n                        x_4911 = res_4907 + res_4909;\n                        x_4912 = res_4910 * res_4910;\n                        x_4913 = x_4888 * x_4912;\n                        x_4914 = x_4891 * x_4913;\n                        y_4915 = x_4914 / res_4901;\n                        res_4916 = x_4911 + y_4915;\n                        y_4917 = res_4901 - 1.0;\n                        res_4918 = res_4916 / y_4917;\n                        res_4898 = res_4905;\n                        res_4899 = res_4901;\n                        res_4900 = res_4918;\n                    }\n                    res_4894 = res_4898;\n                    res_4895 = res_4899;\n                    res_4896 = res_4900;\n                }\n            }\n            x_4887 = res_4894;\n            x_4888 = res_4895;\n            x_4889 = res_4896;\n            *(volatile __local double *) &mem_5456[local_tid_5368 * 8] = x_4887;\n            *(volatile __local double *) &mem_5459[local_tid_5368 * 8] = x_4888;\n            *(volatile __local double *) &mem_5462[local_tid_536",
            "8 * 8] = x_4889;\n        }\n        offset_5572 *= 2;\n        other_index_5261 = local_tid_5368 + offset_5572;\n    }\n    skip_waves_5571 = 1;\n    while (slt32(skip_waves_5571, squot32(max_num_groups_5246 +\n                                          wave_sizze_5566 - 1,\n                                          wave_sizze_5566))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5572 = skip_waves_5571 * wave_sizze_5566;\n        other_index_5261 = local_tid_5368 + offset_5572;\n        if (slt32(other_index_5261, max_num_groups_5246) && ((local_tid_5368 -\n                                                              squot32(local_tid_5368,\n                                                                      wave_sizze_5566) *\n                                                              wave_sizze_5566) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5368,\n                                                                      wave_sizze_5566) &\n                                                              (2 *\n                                                               skip_waves_5571 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4890 = *(__local double *) &mem_5456[(local_tid_5368 +\n                                                        offset_5572) * 8];\n                x_4891 = *(__local double *) &mem_5459[(local_tid_5368 +\n                                                        offset_5572) * 8];\n                x_4892 = *(__local double *) &mem_5462[(local_tid_5368 +\n                                                        offset_5572) * 8];\n            }\n            \n            bool cond_4893;\n            double res_4894;\n            double res_4895;\n            double res_4896;\n            \n            if (thread_active_5568) {\n                cond_4893 = x_4",
            "888 == 0.0;\n                if (cond_4893) {\n                    res_4894 = x_4890;\n                    res_4895 = x_4891;\n                    res_4896 = x_4892;\n                } else {\n                    bool cond_4897;\n                    double res_4898;\n                    double res_4899;\n                    double res_4900;\n                    \n                    cond_4897 = x_4891 == 0.0;\n                    if (cond_4897) {\n                        res_4898 = x_4887;\n                        res_4899 = x_4888;\n                        res_4900 = x_4889;\n                    } else {\n                        double res_4901;\n                        double res_4902;\n                        double res_4903;\n                        double x_4904;\n                        double res_4905;\n                        double y_4906;\n                        double res_4907;\n                        double y_4908;\n                        double res_4909;\n                        double res_4910;\n                        double x_4911;\n                        double x_4912;\n                        double x_4913;\n                        double x_4914;\n                        double y_4915;\n                        double res_4916;\n                        double y_4917;\n                        double res_4918;\n                        \n                        res_4901 = x_4888 + x_4891;\n                        res_4902 = x_4887 * x_4888;\n                        res_4903 = x_4890 * x_4891;\n                        x_4904 = res_4902 + res_4903;\n                        res_4905 = x_4904 / res_4901;\n                        y_4906 = x_4888 - 1.0;\n                        res_4907 = x_4889 * y_4906;\n                        y_4908 = x_4891 - 1.0;\n                        res_4909 = x_4892 * y_4908;\n                        res_4910 = x_4890 - x_4887;\n                        x_4911 = res_4907 + res_4909;\n                        x_4912 = res_4910 * res_4910;\n                        x_4913 = x_4",
            "888 * x_4912;\n                        x_4914 = x_4891 * x_4913;\n                        y_4915 = x_4914 / res_4901;\n                        res_4916 = x_4911 + y_4915;\n                        y_4917 = res_4901 - 1.0;\n                        res_4918 = res_4916 / y_4917;\n                        res_4898 = res_4905;\n                        res_4899 = res_4901;\n                        res_4900 = res_4918;\n                    }\n                    res_4894 = res_4898;\n                    res_4895 = res_4899;\n                    res_4896 = res_4900;\n                }\n            }\n            x_4887 = res_4894;\n            x_4888 = res_4895;\n            x_4889 = res_4896;\n            *(__local double *) &mem_5456[local_tid_5368 * 8] = x_4887;\n            *(__local double *) &mem_5459[local_tid_5368 * 8] = x_4888;\n            *(__local double *) &mem_5462[local_tid_5368 * 8] = x_4889;\n        }\n        skip_waves_5571 *= 2;\n    }\n    final_result_5381 = x_4887;\n    final_result_5382 = x_4888;\n    final_result_5383 = x_4889;\n    if (local_tid_5368 == 0) {\n        *(__global double *) &mem_5465[group_id_5369 * 8] = final_result_5381;\n    }\n    if (local_tid_5368 == 0) {\n        *(__global double *) &mem_5468[group_id_5369 * 8] = final_result_5382;\n    }\n    if (local_tid_5368 == 0) {\n        *(__global double *) &mem_5471[group_id_5369 * 8] = final_result_5383;\n    }\n}\n",
            NULL};
struct memblock_device {
    int *references;
    cl_mem mem;
    int64_t size;
    const char *desc;
} ;
struct memblock_local {
    int *references;
    unsigned char mem;
    int64_t size;
    const char *desc;
} ;
struct memblock {
    int *references;
    char *mem;
    int64_t size;
    const char *desc;
} ;
static const char *size_names[] = {"group_size_4951", "max_num_groups_4953",
                                   "group_size_5012", "max_num_groups_5014",
                                   "group_size_5084", "max_num_groups_5086",
                                   "group_size_5243", "max_num_groups_5245"};
static const char *size_classes[] = {"group_size", "num_groups", "group_size",
                                     "num_groups", "group_size", "num_groups",
                                     "group_size", "num_groups"};
static const char *size_entry_points[] = {"sum", "sum", "mean", "mean",
                                          "variance", "variance", "stddev",
                                          "stddev"};
int futhark_get_num_sizes(void)
{
    return 8;
}
const char *futhark_get_size_name(int i)
{
    return size_names[i];
}
const char *futhark_get_size_class(int i)
{
    return size_classes[i];
}
const char *futhark_get_size_entry(int i)
{
    return size_entry_points[i];
}
struct sizes {
    size_t group_sizze_4951;
    size_t max_num_groups_4953;
    size_t group_sizze_5012;
    size_t max_num_groups_5014;
    size_t group_sizze_5084;
    size_t max_num_groups_5086;
    size_t group_sizze_5243;
    size_t max_num_groups_5245;
} ;
struct futhark_context_config {
    struct opencl_config opencl;
    size_t sizes[8];
} ;
struct futhark_context_config *futhark_context_config_new(void)
{
    struct futhark_context_config *cfg =
                                  malloc(sizeof(struct futhark_context_config));
    
    if (cfg == NULL)
        return NULL;
    cfg->sizes[0] = 0;
    cfg->sizes[1] = 0;
    cfg->sizes[2] = 0;
    cfg->sizes[3] = 0;
    cfg->sizes[4] = 0;
    cfg->sizes[5] = 0;
    cfg->sizes[6] = 0;
    cfg->sizes[7] = 0;
    opencl_config_init(&cfg->opencl, 8, size_names, cfg->sizes, size_classes,
                       size_entry_points);
    cfg->opencl.transpose_block_dim = 16;
    return cfg;
}
void futhark_context_config_free(struct futhark_context_config *cfg)
{
    free(cfg);
}
void futhark_context_config_set_debugging(struct futhark_context_config *cfg,
                                          int flag)
{
    cfg->opencl.logging = cfg->opencl.debugging = flag;
}
void futhark_context_config_set_logging(struct futhark_context_config *cfg,
                                        int flag)
{
    cfg->opencl.logging = flag;
}
void futhark_context_config_set_device(struct futhark_context_config *cfg, const
                                       char *s)
{
    set_preferred_device(&cfg->opencl, s);
}
void futhark_context_config_set_platform(struct futhark_context_config *cfg,
                                         const char *s)
{
    set_preferred_platform(&cfg->opencl, s);
}
void futhark_context_config_dump_program_to(struct futhark_context_config *cfg,
                                            const char *path)
{
    cfg->opencl.dump_program_to = path;
}
void futhark_context_config_load_program_from(struct futhark_context_config *cfg,
                                              const char *path)
{
    cfg->opencl.load_program_from = path;
}
void futhark_context_config_set_default_group_size(struct futhark_context_config *cfg,
                                                   int size)
{
    cfg->opencl.default_group_size = size;
    cfg->opencl.default_group_size_changed = 1;
}
void futhark_context_config_set_default_num_groups(struct futhark_context_config *cfg,
                                                   int num)
{
    cfg->opencl.default_num_groups = num;
}
void futhark_context_config_set_default_tile_size(struct futhark_context_config *cfg,
                                                  int size)
{
    cfg->opencl.default_tile_size = size;
    cfg->opencl.default_tile_size_changed = 1;
}
void futhark_context_config_set_default_threshold(struct futhark_context_config *cfg,
                                                  int size)
{
    cfg->opencl.default_threshold = size;
}
int futhark_context_config_set_size(struct futhark_context_config *cfg, const
                                    char *size_name, size_t size_value)
{
    for (int i = 0; i < 8; i++) {
        if (strcmp(size_name, size_names[i]) == 0) {
            cfg->sizes[i] = size_value;
            return 0;
        }
    }
    return 1;
}
struct futhark_context {
    int detail_memory;
    int debugging;
    int logging;
    lock_t lock;
    char *error;
    int64_t peak_mem_usage_device;
    int64_t cur_mem_usage_device;
    int64_t peak_mem_usage_local;
    int64_t cur_mem_usage_local;
    int64_t peak_mem_usage_default;
    int64_t cur_mem_usage_default;
    int total_runs;
    long total_runtime;
    cl_kernel chunked_reduce_kernel_4968;
    int chunked_reduce_kernel_4968_total_runtime;
    int chunked_reduce_kernel_4968_runs;
    cl_kernel chunked_reduce_kernel_5029;
    int chunked_reduce_kernel_5029_total_runtime;
    int chunked_reduce_kernel_5029_runs;
    cl_kernel chunked_reduce_kernel_5103;
    int chunked_reduce_kernel_5103_total_runtime;
    int chunked_reduce_kernel_5103_runs;
    cl_kernel chunked_reduce_kernel_5262;
    int chunked_reduce_kernel_5262_total_runtime;
    int chunked_reduce_kernel_5262_runs;
    cl_kernel fut_kernel_map_transpose_f64;
    int fut_kernel_map_transpose_f64_total_runtime;
    int fut_kernel_map_transpose_f64_runs;
    cl_kernel fut_kernel_map_transpose_lowheight_f64;
    int fut_kernel_map_transpose_lowheight_f64_total_runtime;
    int fut_kernel_map_transpose_lowheight_f64_runs;
    cl_kernel fut_kernel_map_transpose_lowwidth_f64;
    int fut_kernel_map_transpose_lowwidth_f64_total_runtime;
    int fut_kernel_map_transpose_lowwidth_f64_runs;
    cl_kernel fut_kernel_map_transpose_small_f64;
    int fut_kernel_map_transpose_small_f64_total_runtime;
    int fut_kernel_map_transpose_small_f64_runs;
    cl_kernel reduce_kernel_4996;
    int reduce_kernel_4996_total_runtime;
    int reduce_kernel_4996_runs;
    cl_kernel reduce_kernel_5057;
    int reduce_kernel_5057_total_runtime;
    int reduce_kernel_5057_runs;
    cl_kernel reduce_kernel_5208;
    int reduce_kernel_5208_total_runtime;
    int reduce_kernel_5208_runs;
    cl_kernel reduce_kernel_5367;
    int reduce_kernel_5367_total_runtime;
    int reduce_kernel_5367_runs;
    struct opencl_context opencl;
    struct sizes sizes;
} ;
void post_opencl_setup(struct opencl_context *ctx,
                       struct opencl_device_option *option)
{
    if ((ctx->lockstep_width == 0 && strstr(option->platform_name,
                                            "NVIDIA CUDA") != NULL) &&
        option->device_type == CL_DEVICE_TYPE_GPU)
        ctx->lockstep_width = 32;
    if ((ctx->lockstep_width == 0 && strstr(option->platform_name,
                                            "AMD Accelerated Parallel Processing") !=
         NULL) && option->device_type == CL_DEVICE_TYPE_GPU)
        ctx->lockstep_width = 64;
    if ((ctx->lockstep_width == 0 && strstr(option->platform_name, "") !=
         NULL) && option->device_type == CL_DEVICE_TYPE_GPU)
        ctx->lockstep_width = 1;
    if ((ctx->cfg.default_num_groups == 0 && strstr(option->platform_name,
                                                    "") != NULL) &&
        option->device_type == CL_DEVICE_TYPE_GPU)
        ctx->cfg.default_num_groups = 128;
    if ((ctx->cfg.default_group_size == 0 && strstr(option->platform_name,
                                                    "") != NULL) &&
        option->device_type == CL_DEVICE_TYPE_GPU)
        ctx->cfg.default_group_size = 256;
    if ((ctx->cfg.default_tile_size == 0 && strstr(option->platform_name, "") !=
         NULL) && option->device_type == CL_DEVICE_TYPE_GPU)
        ctx->cfg.default_tile_size = 32;
    if ((ctx->lockstep_width == 0 && strstr(option->platform_name, "") !=
         NULL) && option->device_type == CL_DEVICE_TYPE_CPU)
        ctx->lockstep_width = 1;
    if ((ctx->cfg.default_num_groups == 0 && strstr(option->platform_name,
                                                    "") != NULL) &&
        option->device_type == CL_DEVICE_TYPE_CPU)
        clGetDeviceInfo(ctx->device, CL_DEVICE_MAX_COMPUTE_UNITS,
                        sizeof(ctx->cfg.default_num_groups),
                        &ctx->cfg.default_num_groups, NULL);
    if ((ctx->cfg.default_group_size == 0 && strstr(option->platform_name,
                                                    "") != NULL) &&
        option->device_type == CL_DEVICE_TYPE_CPU)
        ctx->cfg.default_group_size = 32;
    if ((ctx->cfg.default_tile_size == 0 && strstr(option->platform_name, "") !=
         NULL) && option->device_type == CL_DEVICE_TYPE_CPU)
        ctx->cfg.default_tile_size = 4;
}
static void init_context_early(struct futhark_context_config *cfg,
                               struct futhark_context *ctx)
{
    cl_int error;
    
    ctx->opencl.cfg = cfg->opencl;
    ctx->detail_memory = cfg->opencl.debugging;
    ctx->debugging = cfg->opencl.debugging;
    ctx->logging = cfg->opencl.logging;
    ctx->error = NULL;
    create_lock(&ctx->lock);
    ctx->peak_mem_usage_device = 0;
    ctx->cur_mem_usage_device = 0;
    ctx->peak_mem_usage_local = 0;
    ctx->cur_mem_usage_local = 0;
    ctx->peak_mem_usage_default = 0;
    ctx->cur_mem_usage_default = 0;
    ctx->total_runs = 0;
    ctx->total_runtime = 0;
    ctx->chunked_reduce_kernel_4968_total_runtime = 0;
    ctx->chunked_reduce_kernel_4968_runs = 0;
    ctx->chunked_reduce_kernel_5029_total_runtime = 0;
    ctx->chunked_reduce_kernel_5029_runs = 0;
    ctx->chunked_reduce_kernel_5103_total_runtime = 0;
    ctx->chunked_reduce_kernel_5103_runs = 0;
    ctx->chunked_reduce_kernel_5262_total_runtime = 0;
    ctx->chunked_reduce_kernel_5262_runs = 0;
    ctx->fut_kernel_map_transpose_f64_total_runtime = 0;
    ctx->fut_kernel_map_transpose_f64_runs = 0;
    ctx->fut_kernel_map_transpose_lowheight_f64_total_runtime = 0;
    ctx->fut_kernel_map_transpose_lowheight_f64_runs = 0;
    ctx->fut_kernel_map_transpose_lowwidth_f64_total_runtime = 0;
    ctx->fut_kernel_map_transpose_lowwidth_f64_runs = 0;
    ctx->fut_kernel_map_transpose_small_f64_total_runtime = 0;
    ctx->fut_kernel_map_transpose_small_f64_runs = 0;
    ctx->reduce_kernel_4996_total_runtime = 0;
    ctx->reduce_kernel_4996_runs = 0;
    ctx->reduce_kernel_5057_total_runtime = 0;
    ctx->reduce_kernel_5057_runs = 0;
    ctx->reduce_kernel_5208_total_runtime = 0;
    ctx->reduce_kernel_5208_runs = 0;
    ctx->reduce_kernel_5367_total_runtime = 0;
    ctx->reduce_kernel_5367_runs = 0;
}
static void init_context_late(struct futhark_context_config *cfg,
                              struct futhark_context *ctx, cl_program prog)
{
    cl_int error;
    
    {
        ctx->chunked_reduce_kernel_4968 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_4968",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_4968");
    }
    {
        ctx->chunked_reduce_kernel_5029 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5029",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5029");
    }
    {
        ctx->chunked_reduce_kernel_5103 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5103",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5103");
    }
    {
        ctx->chunked_reduce_kernel_5262 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5262",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5262");
    }
    {
        ctx->fut_kernel_map_transpose_f64 = clCreateKernel(prog,
                                                           "fut_kernel_map_transpose_f64",
                                                           &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "fut_kernel_map_transpose_f64");
    }
    {
        ctx->fut_kernel_map_transpose_lowheight_f64 = clCreateKernel(prog,
                                                                     "fut_kernel_map_transpose_lowheight_f64",
                                                                     &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "fut_kernel_map_transpose_lowheight_f64");
    }
    {
        ctx->fut_kernel_map_transpose_lowwidth_f64 = clCreateKernel(prog,
                                                                    "fut_kernel_map_transpose_lowwidth_f64",
                                                                    &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "fut_kernel_map_transpose_lowwidth_f64");
    }
    {
        ctx->fut_kernel_map_transpose_small_f64 = clCreateKernel(prog,
                                                                 "fut_kernel_map_transpose_small_f64",
                                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "fut_kernel_map_transpose_small_f64");
    }
    {
        ctx->reduce_kernel_4996 = clCreateKernel(prog, "reduce_kernel_4996",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_4996");
    }
    {
        ctx->reduce_kernel_5057 = clCreateKernel(prog, "reduce_kernel_5057",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5057");
    }
    {
        ctx->reduce_kernel_5208 = clCreateKernel(prog, "reduce_kernel_5208",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5208");
    }
    {
        ctx->reduce_kernel_5367 = clCreateKernel(prog, "reduce_kernel_5367",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5367");
    }
    ctx->sizes.group_sizze_4951 = cfg->sizes[0];
    ctx->sizes.max_num_groups_4953 = cfg->sizes[1];
    ctx->sizes.group_sizze_5012 = cfg->sizes[2];
    ctx->sizes.max_num_groups_5014 = cfg->sizes[3];
    ctx->sizes.group_sizze_5084 = cfg->sizes[4];
    ctx->sizes.max_num_groups_5086 = cfg->sizes[5];
    ctx->sizes.group_sizze_5243 = cfg->sizes[6];
    ctx->sizes.max_num_groups_5245 = cfg->sizes[7];
}
struct futhark_context *futhark_context_new(struct futhark_context_config *cfg)
{
    struct futhark_context *ctx = malloc(sizeof(struct futhark_context));
    
    if (ctx == NULL)
        return NULL;
    
    int required_types = 0;
    
    required_types |= OPENCL_F64;
    init_context_early(cfg, ctx);
    
    cl_program prog = setup_opencl(&ctx->opencl, opencl_program,
                                   required_types);
    
    init_context_late(cfg, ctx, prog);
    return ctx;
}
struct futhark_context *futhark_context_new_with_command_queue(struct futhark_context_config *cfg,
                                                               cl_command_queue queue)
{
    struct futhark_context *ctx = malloc(sizeof(struct futhark_context));
    
    if (ctx == NULL)
        return NULL;
    
    int required_types = 0;
    
    required_types |= OPENCL_F64;
    init_context_early(cfg, ctx);
    
    cl_program prog = setup_opencl_with_command_queue(&ctx->opencl, queue,
                                                      opencl_program,
                                                      required_types);
    
    init_context_late(cfg, ctx, prog);
    return ctx;
}
void futhark_context_free(struct futhark_context *ctx)
{
    free_lock(&ctx->lock);
    free(ctx);
}
int futhark_context_sync(struct futhark_context *ctx)
{
    OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
    return 0;
}
char *futhark_context_get_error(struct futhark_context *ctx)
{
    char *error = ctx->error;
    
    ctx->error = NULL;
    return error;
}
int futhark_context_clear_caches(struct futhark_context *ctx)
{
    OPENCL_SUCCEED(opencl_free_all(&ctx->opencl));
    return 0;
}
cl_command_queue futhark_context_get_command_queue(struct futhark_context *ctx)
{
    return ctx->opencl.queue;
}
static void memblock_unref_device(struct futhark_context *ctx,
                                  struct memblock_device *block, const
                                  char *desc)
{
    if (block->references != NULL) {
        *block->references -= 1;
        if (ctx->detail_memory)
            fprintf(stderr,
                    "Unreferencing block %s (allocated as %s) in %s: %d references remaining.\n",
                    desc, block->desc, "space 'device'", *block->references);
        if (*block->references == 0) {
            ctx->cur_mem_usage_device -= block->size;
            OPENCL_SUCCEED(opencl_free(&ctx->opencl, block->mem, block->desc));
            free(block->references);
            if (ctx->detail_memory)
                fprintf(stderr,
                        "%lld bytes freed (now allocated: %lld bytes)\n",
                        (long long) block->size,
                        (long long) ctx->cur_mem_usage_device);
        }
        block->references = NULL;
    }
}
static void memblock_alloc_device(struct futhark_context *ctx,
                                  struct memblock_device *block, int64_t size,
                                  const char *desc)
{
    if (size < 0)
        panic(1, "Negative allocation of %lld bytes attempted for %s in %s.\n",
              (long long) size, desc, "space 'device'",
              ctx->cur_mem_usage_device);
    memblock_unref_device(ctx, block, desc);
    OPENCL_SUCCEED(opencl_alloc(&ctx->opencl, size, desc, &block->mem));
    block->references = (int *) malloc(sizeof(int));
    *block->references = 1;
    block->size = size;
    block->desc = desc;
    ctx->cur_mem_usage_device += size;
    if (ctx->detail_memory)
        fprintf(stderr,
                "Allocated %lld bytes for %s in %s (now allocated: %lld bytes)",
                (long long) size, desc, "space 'device'",
                (long long) ctx->cur_mem_usage_device);
    if (ctx->cur_mem_usage_device > ctx->peak_mem_usage_device) {
        ctx->peak_mem_usage_device = ctx->cur_mem_usage_device;
        if (ctx->detail_memory)
            fprintf(stderr, " (new peak).\n");
    } else if (ctx->detail_memory)
        fprintf(stderr, ".\n");
}
static void memblock_set_device(struct futhark_context *ctx,
                                struct memblock_device *lhs,
                                struct memblock_device *rhs, const
                                char *lhs_desc)
{
    memblock_unref_device(ctx, lhs, lhs_desc);
    (*rhs->references)++;
    *lhs = *rhs;
}
static void memblock_unref_local(struct futhark_context *ctx,
                                 struct memblock_local *block, const char *desc)
{
    if (block->references != NULL) {
        *block->references -= 1;
        if (ctx->detail_memory)
            fprintf(stderr,
                    "Unreferencing block %s (allocated as %s) in %s: %d references remaining.\n",
                    desc, block->desc, "space 'local'", *block->references);
        if (*block->references == 0) {
            ctx->cur_mem_usage_local -= block->size;
            free(block->references);
            if (ctx->detail_memory)
                fprintf(stderr,
                        "%lld bytes freed (now allocated: %lld bytes)\n",
                        (long long) block->size,
                        (long long) ctx->cur_mem_usage_local);
        }
        block->references = NULL;
    }
}
static void memblock_alloc_local(struct futhark_context *ctx,
                                 struct memblock_local *block, int64_t size,
                                 const char *desc)
{
    if (size < 0)
        panic(1, "Negative allocation of %lld bytes attempted for %s in %s.\n",
              (long long) size, desc, "space 'local'",
              ctx->cur_mem_usage_local);
    memblock_unref_local(ctx, block, desc);
    block->references = (int *) malloc(sizeof(int));
    *block->references = 1;
    block->size = size;
    block->desc = desc;
    ctx->cur_mem_usage_local += size;
    if (ctx->detail_memory)
        fprintf(stderr,
                "Allocated %lld bytes for %s in %s (now allocated: %lld bytes)",
                (long long) size, desc, "space 'local'",
                (long long) ctx->cur_mem_usage_local);
    if (ctx->cur_mem_usage_local > ctx->peak_mem_usage_local) {
        ctx->peak_mem_usage_local = ctx->cur_mem_usage_local;
        if (ctx->detail_memory)
            fprintf(stderr, " (new peak).\n");
    } else if (ctx->detail_memory)
        fprintf(stderr, ".\n");
}
static void memblock_set_local(struct futhark_context *ctx,
                               struct memblock_local *lhs,
                               struct memblock_local *rhs, const char *lhs_desc)
{
    memblock_unref_local(ctx, lhs, lhs_desc);
    (*rhs->references)++;
    *lhs = *rhs;
}
static void memblock_unref(struct futhark_context *ctx, struct memblock *block,
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
static void memblock_alloc(struct futhark_context *ctx, struct memblock *block,
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
static void memblock_set(struct futhark_context *ctx, struct memblock *lhs,
                         struct memblock *rhs, const char *lhs_desc)
{
    memblock_unref(ctx, lhs, lhs_desc);
    (*rhs->references)++;
    *lhs = *rhs;
}
void futhark_debugging_report(struct futhark_context *ctx)
{
    if (ctx->detail_memory) {
        fprintf(stderr, "Peak memory usage for space 'device': %lld bytes.\n",
                (long long) ctx->peak_mem_usage_device);
        fprintf(stderr, "Peak memory usage for space 'local': %lld bytes.\n",
                (long long) ctx->peak_mem_usage_local);
        fprintf(stderr, "Peak memory usage for default space: %lld bytes.\n",
                (long long) ctx->peak_mem_usage_default);
    }
    if (ctx->debugging) {
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_4968             executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_4968_runs,
                (long) ctx->chunked_reduce_kernel_4968_total_runtime /
                (ctx->chunked_reduce_kernel_4968_runs !=
                 0 ? ctx->chunked_reduce_kernel_4968_runs : 1),
                (long) ctx->chunked_reduce_kernel_4968_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_4968_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_4968_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5029             executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5029_runs,
                (long) ctx->chunked_reduce_kernel_5029_total_runtime /
                (ctx->chunked_reduce_kernel_5029_runs !=
                 0 ? ctx->chunked_reduce_kernel_5029_runs : 1),
                (long) ctx->chunked_reduce_kernel_5029_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5029_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5029_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5103             executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5103_runs,
                (long) ctx->chunked_reduce_kernel_5103_total_runtime /
                (ctx->chunked_reduce_kernel_5103_runs !=
                 0 ? ctx->chunked_reduce_kernel_5103_runs : 1),
                (long) ctx->chunked_reduce_kernel_5103_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5103_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5103_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5262             executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5262_runs,
                (long) ctx->chunked_reduce_kernel_5262_total_runtime /
                (ctx->chunked_reduce_kernel_5262_runs !=
                 0 ? ctx->chunked_reduce_kernel_5262_runs : 1),
                (long) ctx->chunked_reduce_kernel_5262_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5262_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5262_runs;
        fprintf(stderr,
                "Kernel fut_kernel_map_transpose_f64           executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->fut_kernel_map_transpose_f64_runs,
                (long) ctx->fut_kernel_map_transpose_f64_total_runtime /
                (ctx->fut_kernel_map_transpose_f64_runs !=
                 0 ? ctx->fut_kernel_map_transpose_f64_runs : 1),
                (long) ctx->fut_kernel_map_transpose_f64_total_runtime);
        ctx->total_runtime += ctx->fut_kernel_map_transpose_f64_total_runtime;
        ctx->total_runs += ctx->fut_kernel_map_transpose_f64_runs;
        fprintf(stderr,
                "Kernel fut_kernel_map_transpose_lowheight_f64 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->fut_kernel_map_transpose_lowheight_f64_runs,
                (long) ctx->fut_kernel_map_transpose_lowheight_f64_total_runtime /
                (ctx->fut_kernel_map_transpose_lowheight_f64_runs !=
                 0 ? ctx->fut_kernel_map_transpose_lowheight_f64_runs : 1),
                (long) ctx->fut_kernel_map_transpose_lowheight_f64_total_runtime);
        ctx->total_runtime +=
            ctx->fut_kernel_map_transpose_lowheight_f64_total_runtime;
        ctx->total_runs += ctx->fut_kernel_map_transpose_lowheight_f64_runs;
        fprintf(stderr,
                "Kernel fut_kernel_map_transpose_lowwidth_f64  executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->fut_kernel_map_transpose_lowwidth_f64_runs,
                (long) ctx->fut_kernel_map_transpose_lowwidth_f64_total_runtime /
                (ctx->fut_kernel_map_transpose_lowwidth_f64_runs !=
                 0 ? ctx->fut_kernel_map_transpose_lowwidth_f64_runs : 1),
                (long) ctx->fut_kernel_map_transpose_lowwidth_f64_total_runtime);
        ctx->total_runtime +=
            ctx->fut_kernel_map_transpose_lowwidth_f64_total_runtime;
        ctx->total_runs += ctx->fut_kernel_map_transpose_lowwidth_f64_runs;
        fprintf(stderr,
                "Kernel fut_kernel_map_transpose_small_f64     executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->fut_kernel_map_transpose_small_f64_runs,
                (long) ctx->fut_kernel_map_transpose_small_f64_total_runtime /
                (ctx->fut_kernel_map_transpose_small_f64_runs !=
                 0 ? ctx->fut_kernel_map_transpose_small_f64_runs : 1),
                (long) ctx->fut_kernel_map_transpose_small_f64_total_runtime);
        ctx->total_runtime +=
            ctx->fut_kernel_map_transpose_small_f64_total_runtime;
        ctx->total_runs += ctx->fut_kernel_map_transpose_small_f64_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_4996                     executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_4996_runs,
                (long) ctx->reduce_kernel_4996_total_runtime /
                (ctx->reduce_kernel_4996_runs !=
                 0 ? ctx->reduce_kernel_4996_runs : 1),
                (long) ctx->reduce_kernel_4996_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_4996_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_4996_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5057                     executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5057_runs,
                (long) ctx->reduce_kernel_5057_total_runtime /
                (ctx->reduce_kernel_5057_runs !=
                 0 ? ctx->reduce_kernel_5057_runs : 1),
                (long) ctx->reduce_kernel_5057_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5057_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5057_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5208                     executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5208_runs,
                (long) ctx->reduce_kernel_5208_total_runtime /
                (ctx->reduce_kernel_5208_runs !=
                 0 ? ctx->reduce_kernel_5208_runs : 1),
                (long) ctx->reduce_kernel_5208_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5208_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5208_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5367                     executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5367_runs,
                (long) ctx->reduce_kernel_5367_total_runtime /
                (ctx->reduce_kernel_5367_runs !=
                 0 ? ctx->reduce_kernel_5367_runs : 1),
                (long) ctx->reduce_kernel_5367_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5367_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5367_runs;
        if (ctx->debugging)
            fprintf(stderr, "Ran %d kernels with cumulative runtime: %6ldus\n",
                    ctx->total_runs, ctx->total_runtime);
    }
}
static int futrts_map_transpose_opencl_f64(struct futhark_context *ctx,
                                           struct memblock_device destmem_0,
                                           int32_t destoffset_1,
                                           struct memblock_device srcmem_2,
                                           int32_t srcoffset_3,
                                           int32_t num_arrays_4,
                                           int32_t x_elems_5, int32_t y_elems_6,
                                           int32_t in_elems_7,
                                           int32_t out_elems_8);
static int futrts_sum(struct futhark_context *ctx, double *out_scalar_out_5596,
                      int64_t col_mem_sizze_5424,
                      struct memblock_device col_mem_5425, int32_t sizze_4805);
static int futrts_mean(struct futhark_context *ctx, double *out_scalar_out_5608,
                       int64_t col_mem_sizze_5424,
                       struct memblock_device col_mem_5425, int32_t sizze_4812);
static int futrts_variance(struct futhark_context *ctx,
                           double *out_scalar_out_5620,
                           int64_t values_mem_sizze_5424,
                           struct memblock_device values_mem_5425,
                           int32_t sizze_4821);
static int futrts_stddev(struct futhark_context *ctx,
                         double *out_scalar_out_5633,
                         int64_t values_mem_sizze_5424,
                         struct memblock_device values_mem_5425,
                         int32_t sizze_4882);
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
static int futrts_map_transpose_opencl_f64(struct futhark_context *ctx,
                                           struct memblock_device destmem_0,
                                           int32_t destoffset_1,
                                           struct memblock_device srcmem_2,
                                           int32_t srcoffset_3,
                                           int32_t num_arrays_4,
                                           int32_t x_elems_5, int32_t y_elems_6,
                                           int32_t in_elems_7,
                                           int32_t out_elems_8)
{
    if (!(num_arrays_4 * x_elems_5 * y_elems_6 == 0)) {
        if (in_elems_7 == out_elems_8 && ((num_arrays_4 == 1 || x_elems_5 *
                                           y_elems_6 == in_elems_7) &&
                                          (x_elems_5 == 1 || y_elems_6 == 1))) {
            if (in_elems_7 * sizeof(double) > 0) {
                OPENCL_SUCCEED(clEnqueueCopyBuffer(ctx->opencl.queue,
                                                   srcmem_2.mem, destmem_0.mem,
                                                   srcoffset_3, destoffset_1,
                                                   in_elems_7 * sizeof(double),
                                                   0, NULL, NULL));
                if (ctx->debugging)
                    OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            }
        } else {
            if (sle32(x_elems_5, squot32(16, 2)) && slt32(16, y_elems_6)) {
                int32_t muly_9 = squot32(16, x_elems_5);
                int32_t new_height_10;
                
                new_height_10 = squot32(y_elems_6 + muly_9 - 1, muly_9);
                OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowwidth_f64,
                                              0, sizeof(destmem_0.mem),
                                              &destmem_0.mem));
                OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowwidth_f64,
                                              1, sizeof(destoffset_1),
                                              &destoffset_1));
                OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowwidth_f64,
                                              2, sizeof(srcmem_2.mem),
                                              &srcmem_2.mem));
                OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowwidth_f64,
                                              3, sizeof(srcoffset_3),
                                              &srcoffset_3));
                OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowwidth_f64,
                                              4, sizeof(x_elems_5),
                                              &x_elems_5));
                OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowwidth_f64,
                                              5, sizeof(y_elems_6),
                                              &y_elems_6));
                OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowwidth_f64,
                                              6, sizeof(in_elems_7),
                                              &in_elems_7));
                OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowwidth_f64,
                                              7, sizeof(out_elems_8),
                                              &out_elems_8));
                OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowwidth_f64,
                                              8, sizeof(muly_9), &muly_9));
                OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowwidth_f64,
                                              9, 272 * sizeof(double), NULL));
                if (1 * (x_elems_5 + srem32(16 - srem32(x_elems_5, 16), 16)) *
                    (new_height_10 + srem32(16 - srem32(new_height_10, 16),
                                            16)) * num_arrays_4 != 0) {
                    const size_t global_work_sizze_5576[3] = {x_elems_5 +
                                                              srem32(16 -
                                                                     srem32(x_elems_5,
                                                                            16),
                                                                     16),
                                                              new_height_10 +
                                                              srem32(16 -
                                                                     srem32(new_height_10,
                                                                            16),
                                                                     16),
                                                              num_arrays_4};
                    const size_t local_work_sizze_5580[3] = {16, 16, 1};
                    int64_t time_start_5577 = 0, time_end_5578 = 0;
                    
                    if (ctx->debugging) {
                        fprintf(stderr, "Launching %s with global work size [",
                                "fut_kernel_map_transpose_lowwidth_f64");
                        fprintf(stderr, "%zu", global_work_sizze_5576[0]);
                        fprintf(stderr, ", ");
                        fprintf(stderr, "%zu", global_work_sizze_5576[1]);
                        fprintf(stderr, ", ");
                        fprintf(stderr, "%zu", global_work_sizze_5576[2]);
                        fprintf(stderr, "] and local work size [");
                        fprintf(stderr, "%zu", local_work_sizze_5580[0]);
                        fprintf(stderr, ", ");
                        fprintf(stderr, "%zu", local_work_sizze_5580[1]);
                        fprintf(stderr, ", ");
                        fprintf(stderr, "%zu", local_work_sizze_5580[2]);
                        fprintf(stderr, "].\n");
                        time_start_5577 = get_wall_time();
                    }
                    OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                                          ctx->fut_kernel_map_transpose_lowwidth_f64,
                                                          3, NULL,
                                                          global_work_sizze_5576,
                                                          local_work_sizze_5580,
                                                          0, NULL, NULL));
                    if (ctx->debugging) {
                        OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
                        time_end_5578 = get_wall_time();
                        
                        long time_diff_5579 = time_end_5578 - time_start_5577;
                        
                        ctx->fut_kernel_map_transpose_lowwidth_f64_total_runtime +=
                            time_diff_5579;
                        ctx->fut_kernel_map_transpose_lowwidth_f64_runs++;
                        fprintf(stderr, "kernel %s runtime: %ldus\n",
                                "fut_kernel_map_transpose_lowwidth_f64",
                                time_diff_5579);
                    }
                }
            } else {
                if (sle32(y_elems_6, squot32(16, 2)) && slt32(16, x_elems_5)) {
                    int32_t mulx_11 = squot32(16, y_elems_6);
                    int32_t new_width_12;
                    
                    new_width_12 = squot32(x_elems_5 + mulx_11 - 1, mulx_11);
                    OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowheight_f64,
                                                  0, sizeof(destmem_0.mem),
                                                  &destmem_0.mem));
                    OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowheight_f64,
                                                  1, sizeof(destoffset_1),
                                                  &destoffset_1));
                    OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowheight_f64,
                                                  2, sizeof(srcmem_2.mem),
                                                  &srcmem_2.mem));
                    OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowheight_f64,
                                                  3, sizeof(srcoffset_3),
                                                  &srcoffset_3));
                    OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowheight_f64,
                                                  4, sizeof(x_elems_5),
                                                  &x_elems_5));
                    OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowheight_f64,
                                                  5, sizeof(y_elems_6),
                                                  &y_elems_6));
                    OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowheight_f64,
                                                  6, sizeof(in_elems_7),
                                                  &in_elems_7));
                    OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowheight_f64,
                                                  7, sizeof(out_elems_8),
                                                  &out_elems_8));
                    OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowheight_f64,
                                                  8, sizeof(mulx_11),
                                                  &mulx_11));
                    OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_lowheight_f64,
                                                  9, 272 * sizeof(double),
                                                  NULL));
                    if (1 * (new_width_12 + srem32(16 - srem32(new_width_12,
                                                               16), 16)) *
                        (y_elems_6 + srem32(16 - srem32(y_elems_6, 16), 16)) *
                        num_arrays_4 != 0) {
                        const size_t global_work_sizze_5581[3] = {new_width_12 +
                                                                  srem32(16 -
                                                                         srem32(new_width_12,
                                                                                16),
                                                                         16),
                                                                  y_elems_6 +
                                                                  srem32(16 -
                                                                         srem32(y_elems_6,
                                                                                16),
                                                                         16),
                                                                  num_arrays_4};
                        const size_t local_work_sizze_5585[3] = {16, 16, 1};
                        int64_t time_start_5582 = 0, time_end_5583 = 0;
                        
                        if (ctx->debugging) {
                            fprintf(stderr,
                                    "Launching %s with global work size [",
                                    "fut_kernel_map_transpose_lowheight_f64");
                            fprintf(stderr, "%zu", global_work_sizze_5581[0]);
                            fprintf(stderr, ", ");
                            fprintf(stderr, "%zu", global_work_sizze_5581[1]);
                            fprintf(stderr, ", ");
                            fprintf(stderr, "%zu", global_work_sizze_5581[2]);
                            fprintf(stderr, "] and local work size [");
                            fprintf(stderr, "%zu", local_work_sizze_5585[0]);
                            fprintf(stderr, ", ");
                            fprintf(stderr, "%zu", local_work_sizze_5585[1]);
                            fprintf(stderr, ", ");
                            fprintf(stderr, "%zu", local_work_sizze_5585[2]);
                            fprintf(stderr, "].\n");
                            time_start_5582 = get_wall_time();
                        }
                        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                                              ctx->fut_kernel_map_transpose_lowheight_f64,
                                                              3, NULL,
                                                              global_work_sizze_5581,
                                                              local_work_sizze_5585,
                                                              0, NULL, NULL));
                        if (ctx->debugging) {
                            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
                            time_end_5583 = get_wall_time();
                            
                            long time_diff_5584 = time_end_5583 -
                                 time_start_5582;
                            
                            ctx->fut_kernel_map_transpose_lowheight_f64_total_runtime +=
                                time_diff_5584;
                            ctx->fut_kernel_map_transpose_lowheight_f64_runs++;
                            fprintf(stderr, "kernel %s runtime: %ldus\n",
                                    "fut_kernel_map_transpose_lowheight_f64",
                                    time_diff_5584);
                        }
                    }
                } else {
                    if (sle32(x_elems_5, squot32(16, 2)) && sle32(y_elems_6,
                                                                  squot32(16,
                                                                          2))) {
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_small_f64,
                                                      0, sizeof(destmem_0.mem),
                                                      &destmem_0.mem));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_small_f64,
                                                      1, sizeof(destoffset_1),
                                                      &destoffset_1));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_small_f64,
                                                      2, sizeof(srcmem_2.mem),
                                                      &srcmem_2.mem));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_small_f64,
                                                      3, sizeof(srcoffset_3),
                                                      &srcoffset_3));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_small_f64,
                                                      4, sizeof(num_arrays_4),
                                                      &num_arrays_4));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_small_f64,
                                                      5, sizeof(x_elems_5),
                                                      &x_elems_5));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_small_f64,
                                                      6, sizeof(y_elems_6),
                                                      &y_elems_6));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_small_f64,
                                                      7, sizeof(in_elems_7),
                                                      &in_elems_7));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_small_f64,
                                                      8, sizeof(out_elems_8),
                                                      &out_elems_8));
                        if (1 * (num_arrays_4 * x_elems_5 * y_elems_6 +
                                 srem32(256 - srem32(num_arrays_4 * x_elems_5 *
                                                     y_elems_6, 256), 256)) !=
                            0) {
                            const size_t global_work_sizze_5586[1] =
                                         {num_arrays_4 * x_elems_5 * y_elems_6 +
                                         srem32(256 - srem32(num_arrays_4 *
                                                             x_elems_5 *
                                                             y_elems_6, 256),
                                                256)};
                            const size_t local_work_sizze_5590[1] = {256};
                            int64_t time_start_5587 = 0, time_end_5588 = 0;
                            
                            if (ctx->debugging) {
                                fprintf(stderr,
                                        "Launching %s with global work size [",
                                        "fut_kernel_map_transpose_small_f64");
                                fprintf(stderr, "%zu",
                                        global_work_sizze_5586[0]);
                                fprintf(stderr, "] and local work size [");
                                fprintf(stderr, "%zu",
                                        local_work_sizze_5590[0]);
                                fprintf(stderr, "].\n");
                                time_start_5587 = get_wall_time();
                            }
                            OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                                                  ctx->fut_kernel_map_transpose_small_f64,
                                                                  1, NULL,
                                                                  global_work_sizze_5586,
                                                                  local_work_sizze_5590,
                                                                  0, NULL,
                                                                  NULL));
                            if (ctx->debugging) {
                                OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
                                time_end_5588 = get_wall_time();
                                
                                long time_diff_5589 = time_end_5588 -
                                     time_start_5587;
                                
                                ctx->fut_kernel_map_transpose_small_f64_total_runtime +=
                                    time_diff_5589;
                                ctx->fut_kernel_map_transpose_small_f64_runs++;
                                fprintf(stderr, "kernel %s runtime: %ldus\n",
                                        "fut_kernel_map_transpose_small_f64",
                                        time_diff_5589);
                            }
                        }
                    } else {
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_f64,
                                                      0, sizeof(destmem_0.mem),
                                                      &destmem_0.mem));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_f64,
                                                      1, sizeof(destoffset_1),
                                                      &destoffset_1));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_f64,
                                                      2, sizeof(srcmem_2.mem),
                                                      &srcmem_2.mem));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_f64,
                                                      3, sizeof(srcoffset_3),
                                                      &srcoffset_3));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_f64,
                                                      4, sizeof(x_elems_5),
                                                      &x_elems_5));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_f64,
                                                      5, sizeof(y_elems_6),
                                                      &y_elems_6));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_f64,
                                                      6, sizeof(in_elems_7),
                                                      &in_elems_7));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_f64,
                                                      7, sizeof(out_elems_8),
                                                      &out_elems_8));
                        OPENCL_SUCCEED(clSetKernelArg(ctx->fut_kernel_map_transpose_f64,
                                                      8, 272 * sizeof(double),
                                                      NULL));
                        if (1 * (x_elems_5 + srem32(16 - srem32(x_elems_5, 16),
                                                    16)) * (y_elems_6 +
                                                            srem32(16 -
                                                                   srem32(y_elems_6,
                                                                          16),
                                                                   16)) *
                            num_arrays_4 != 0) {
                            const size_t global_work_sizze_5591[3] =
                                         {x_elems_5 + srem32(16 -
                                                             srem32(x_elems_5,
                                                                    16), 16),
                                          y_elems_6 + srem32(16 -
                                                             srem32(y_elems_6,
                                                                    16), 16),
                                          num_arrays_4};
                            const size_t local_work_sizze_5595[3] = {16, 16, 1};
                            int64_t time_start_5592 = 0, time_end_5593 = 0;
                            
                            if (ctx->debugging) {
                                fprintf(stderr,
                                        "Launching %s with global work size [",
                                        "fut_kernel_map_transpose_f64");
                                fprintf(stderr, "%zu",
                                        global_work_sizze_5591[0]);
                                fprintf(stderr, ", ");
                                fprintf(stderr, "%zu",
                                        global_work_sizze_5591[1]);
                                fprintf(stderr, ", ");
                                fprintf(stderr, "%zu",
                                        global_work_sizze_5591[2]);
                                fprintf(stderr, "] and local work size [");
                                fprintf(stderr, "%zu",
                                        local_work_sizze_5595[0]);
                                fprintf(stderr, ", ");
                                fprintf(stderr, "%zu",
                                        local_work_sizze_5595[1]);
                                fprintf(stderr, ", ");
                                fprintf(stderr, "%zu",
                                        local_work_sizze_5595[2]);
                                fprintf(stderr, "].\n");
                                time_start_5592 = get_wall_time();
                            }
                            OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                                                  ctx->fut_kernel_map_transpose_f64,
                                                                  3, NULL,
                                                                  global_work_sizze_5591,
                                                                  local_work_sizze_5595,
                                                                  0, NULL,
                                                                  NULL));
                            if (ctx->debugging) {
                                OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
                                time_end_5593 = get_wall_time();
                                
                                long time_diff_5594 = time_end_5593 -
                                     time_start_5592;
                                
                                ctx->fut_kernel_map_transpose_f64_total_runtime +=
                                    time_diff_5594;
                                ctx->fut_kernel_map_transpose_f64_runs++;
                                fprintf(stderr, "kernel %s runtime: %ldus\n",
                                        "fut_kernel_map_transpose_f64",
                                        time_diff_5594);
                            }
                        }
                    }
                }
            }
        }
    }
    return 0;
}
static int futrts_sum(struct futhark_context *ctx, double *out_scalar_out_5596,
                      int64_t col_mem_sizze_5424,
                      struct memblock_device col_mem_5425, int32_t sizze_4805)
{
    double scalar_out_5480;
    int32_t group_sizze_4952;
    
    group_sizze_4952 = ctx->sizes.group_sizze_4951;
    
    int32_t max_num_groups_4954;
    
    max_num_groups_4954 = ctx->sizes.max_num_groups_4953;
    
    int32_t y_4955 = group_sizze_4952 - 1;
    int32_t x_4956 = sizze_4805 + y_4955;
    int32_t w_div_group_sizze_4957 = squot32(x_4956, group_sizze_4952);
    int32_t num_groups_maybe_zzero_4958 = smin32(max_num_groups_4954,
                                                 w_div_group_sizze_4957);
    int32_t num_groups_4959 = smax32(1, num_groups_maybe_zzero_4958);
    int32_t num_threads_4960 = group_sizze_4952 * num_groups_4959;
    int32_t y_4961 = num_threads_4960 - 1;
    int32_t x_4962 = sizze_4805 + y_4961;
    int32_t per_thread_elements_4963 = squot32(x_4962, num_threads_4960);
    int64_t binop_x_5430 = sext_i32_i64(num_groups_4959);
    int64_t bytes_5429 = 8 * binop_x_5430;
    struct memblock_device mem_5431;
    
    mem_5431.references = NULL;
    memblock_alloc_device(ctx, &mem_5431, bytes_5429, "mem_5431");
    
    int64_t binop_x_5427 = sext_i32_i64(group_sizze_4952);
    int64_t bytes_5426 = 8 * binop_x_5427;
    struct memblock_local mem_5428;
    
    mem_5428.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4805);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4968, 0,
                                  bytes_5426, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4968, 1,
                                  sizeof(sizze_4805), &sizze_4805));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4968, 2,
                                  sizeof(num_threads_4960), &num_threads_4960));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4968, 3,
                                  sizeof(per_thread_elements_4963),
                                  &per_thread_elements_4963));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4968, 4,
                                  sizeof(col_mem_5425.mem), &col_mem_5425.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4968, 5,
                                  sizeof(mem_5431.mem), &mem_5431.mem));
    if (1 * (num_groups_4959 * group_sizze_4952) != 0) {
        const size_t global_work_sizze_5597[1] = {num_groups_4959 *
                     group_sizze_4952};
        const size_t local_work_sizze_5601[1] = {group_sizze_4952};
        int64_t time_start_5598 = 0, time_end_5599 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_4968");
            fprintf(stderr, "%zu", global_work_sizze_5597[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5601[0]);
            fprintf(stderr, "].\n");
            time_start_5598 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_4968,
                                              1, NULL, global_work_sizze_5597,
                                              local_work_sizze_5601, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5599 = get_wall_time();
            
            long time_diff_5600 = time_end_5599 - time_start_5598;
            
            ctx->chunked_reduce_kernel_4968_total_runtime += time_diff_5600;
            ctx->chunked_reduce_kernel_4968_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_4968", time_diff_5600);
        }
    }
    memblock_unref_local(ctx, &mem_5428, "mem_5428");
    
    struct memblock_device mem_5437;
    
    mem_5437.references = NULL;
    memblock_alloc_device(ctx, &mem_5437, 8, "mem_5437");
    
    int64_t binop_x_5433 = sext_i32_i64(max_num_groups_4954);
    int64_t bytes_5432 = 8 * binop_x_5433;
    struct memblock_local mem_5434;
    
    mem_5434.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_4996, 0, bytes_5432,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_4996, 1,
                                  sizeof(num_groups_4959), &num_groups_4959));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_4996, 2,
                                  sizeof(mem_5431.mem), &mem_5431.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_4996, 3,
                                  sizeof(mem_5437.mem), &mem_5437.mem));
    if (1 * max_num_groups_4954 != 0) {
        const size_t global_work_sizze_5602[1] = {max_num_groups_4954};
        const size_t local_work_sizze_5606[1] = {max_num_groups_4954};
        int64_t time_start_5603 = 0, time_end_5604 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_4996");
            fprintf(stderr, "%zu", global_work_sizze_5602[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5606[0]);
            fprintf(stderr, "].\n");
            time_start_5603 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_4996, 1, NULL,
                                              global_work_sizze_5602,
                                              local_work_sizze_5606, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5604 = get_wall_time();
            
            long time_diff_5605 = time_end_5604 - time_start_5603;
            
            ctx->reduce_kernel_4996_total_runtime += time_diff_5605;
            ctx->reduce_kernel_4996_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_4996",
                    time_diff_5605);
        }
    }
    memblock_unref_device(ctx, &mem_5431, "mem_5431");
    memblock_unref_local(ctx, &mem_5434, "mem_5434");
    
    double read_res_5607;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5437.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5607, 0,
                                       NULL, NULL));
    
    double res_4807 = read_res_5607;
    
    memblock_unref_device(ctx, &mem_5437, "mem_5437");
    scalar_out_5480 = res_4807;
    *out_scalar_out_5596 = scalar_out_5480;
    memblock_unref_local(ctx, &mem_5434, "mem_5434");
    memblock_unref_device(ctx, &mem_5437, "mem_5437");
    memblock_unref_local(ctx, &mem_5428, "mem_5428");
    memblock_unref_device(ctx, &mem_5431, "mem_5431");
    return 0;
}
static int futrts_mean(struct futhark_context *ctx, double *out_scalar_out_5608,
                       int64_t col_mem_sizze_5424,
                       struct memblock_device col_mem_5425, int32_t sizze_4812)
{
    double scalar_out_5498;
    int32_t group_sizze_5013;
    
    group_sizze_5013 = ctx->sizes.group_sizze_5012;
    
    int32_t max_num_groups_5015;
    
    max_num_groups_5015 = ctx->sizes.max_num_groups_5014;
    
    int32_t y_5016 = group_sizze_5013 - 1;
    int32_t x_5017 = sizze_4812 + y_5016;
    int32_t w_div_group_sizze_5018 = squot32(x_5017, group_sizze_5013);
    int32_t num_groups_maybe_zzero_5019 = smin32(max_num_groups_5015,
                                                 w_div_group_sizze_5018);
    int32_t num_groups_5020 = smax32(1, num_groups_maybe_zzero_5019);
    int32_t num_threads_5021 = group_sizze_5013 * num_groups_5020;
    int32_t y_5022 = num_threads_5021 - 1;
    int32_t x_5023 = sizze_4812 + y_5022;
    int32_t per_thread_elements_5024 = squot32(x_5023, num_threads_5021);
    int64_t binop_x_5430 = sext_i32_i64(num_groups_5020);
    int64_t bytes_5429 = 8 * binop_x_5430;
    struct memblock_device mem_5431;
    
    mem_5431.references = NULL;
    memblock_alloc_device(ctx, &mem_5431, bytes_5429, "mem_5431");
    
    int64_t binop_x_5427 = sext_i32_i64(group_sizze_5013);
    int64_t bytes_5426 = 8 * binop_x_5427;
    struct memblock_local mem_5428;
    
    mem_5428.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4812);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5029, 0,
                                  bytes_5426, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5029, 1,
                                  sizeof(sizze_4812), &sizze_4812));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5029, 2,
                                  sizeof(num_threads_5021), &num_threads_5021));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5029, 3,
                                  sizeof(per_thread_elements_5024),
                                  &per_thread_elements_5024));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5029, 4,
                                  sizeof(col_mem_5425.mem), &col_mem_5425.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5029, 5,
                                  sizeof(mem_5431.mem), &mem_5431.mem));
    if (1 * (num_groups_5020 * group_sizze_5013) != 0) {
        const size_t global_work_sizze_5609[1] = {num_groups_5020 *
                     group_sizze_5013};
        const size_t local_work_sizze_5613[1] = {group_sizze_5013};
        int64_t time_start_5610 = 0, time_end_5611 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5029");
            fprintf(stderr, "%zu", global_work_sizze_5609[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5613[0]);
            fprintf(stderr, "].\n");
            time_start_5610 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5029,
                                              1, NULL, global_work_sizze_5609,
                                              local_work_sizze_5613, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5611 = get_wall_time();
            
            long time_diff_5612 = time_end_5611 - time_start_5610;
            
            ctx->chunked_reduce_kernel_5029_total_runtime += time_diff_5612;
            ctx->chunked_reduce_kernel_5029_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5029", time_diff_5612);
        }
    }
    memblock_unref_local(ctx, &mem_5428, "mem_5428");
    
    struct memblock_device mem_5437;
    
    mem_5437.references = NULL;
    memblock_alloc_device(ctx, &mem_5437, 8, "mem_5437");
    
    int64_t binop_x_5433 = sext_i32_i64(max_num_groups_5015);
    int64_t bytes_5432 = 8 * binop_x_5433;
    struct memblock_local mem_5434;
    
    mem_5434.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5057, 0, bytes_5432,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5057, 1,
                                  sizeof(num_groups_5020), &num_groups_5020));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5057, 2,
                                  sizeof(mem_5431.mem), &mem_5431.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5057, 3,
                                  sizeof(mem_5437.mem), &mem_5437.mem));
    if (1 * max_num_groups_5015 != 0) {
        const size_t global_work_sizze_5614[1] = {max_num_groups_5015};
        const size_t local_work_sizze_5618[1] = {max_num_groups_5015};
        int64_t time_start_5615 = 0, time_end_5616 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5057");
            fprintf(stderr, "%zu", global_work_sizze_5614[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5618[0]);
            fprintf(stderr, "].\n");
            time_start_5615 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5057, 1, NULL,
                                              global_work_sizze_5614,
                                              local_work_sizze_5618, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5616 = get_wall_time();
            
            long time_diff_5617 = time_end_5616 - time_start_5615;
            
            ctx->reduce_kernel_5057_total_runtime += time_diff_5617;
            ctx->reduce_kernel_5057_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5057",
                    time_diff_5617);
        }
    }
    memblock_unref_device(ctx, &mem_5431, "mem_5431");
    memblock_unref_local(ctx, &mem_5434, "mem_5434");
    
    double read_res_5619;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5437.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5619, 0,
                                       NULL, NULL));
    
    double res_4814 = read_res_5619;
    
    memblock_unref_device(ctx, &mem_5437, "mem_5437");
    
    double res_4819 = sitofp_i32_f64(sizze_4812);
    double res_4820 = res_4814 / res_4819;
    
    scalar_out_5498 = res_4820;
    *out_scalar_out_5608 = scalar_out_5498;
    memblock_unref_local(ctx, &mem_5434, "mem_5434");
    memblock_unref_device(ctx, &mem_5437, "mem_5437");
    memblock_unref_local(ctx, &mem_5428, "mem_5428");
    memblock_unref_device(ctx, &mem_5431, "mem_5431");
    return 0;
}
static int futrts_variance(struct futhark_context *ctx,
                           double *out_scalar_out_5620,
                           int64_t values_mem_sizze_5424,
                           struct memblock_device values_mem_5425,
                           int32_t sizze_4821)
{
    double scalar_out_5516;
    int32_t group_sizze_5085;
    
    group_sizze_5085 = ctx->sizes.group_sizze_5084;
    
    int32_t max_num_groups_5087;
    
    max_num_groups_5087 = ctx->sizes.max_num_groups_5086;
    
    int32_t y_5088 = group_sizze_5085 - 1;
    int32_t x_5089 = sizze_4821 + y_5088;
    int32_t w_div_group_sizze_5090 = squot32(x_5089, group_sizze_5085);
    int32_t num_groups_maybe_zzero_5091 = smin32(max_num_groups_5087,
                                                 w_div_group_sizze_5090);
    int32_t num_groups_5092 = smax32(1, num_groups_maybe_zzero_5091);
    int32_t num_threads_5093 = group_sizze_5085 * num_groups_5092;
    int32_t y_5094 = num_threads_5093 - 1;
    int32_t x_5095 = sizze_4821 + y_5094;
    int32_t per_thread_elements_5096 = squot32(x_5095, num_threads_5093);
    int32_t y_5390 = smod32(sizze_4821, num_threads_5093);
    int32_t x_5391 = num_threads_5093 - y_5390;
    int32_t y_5392 = smod32(x_5391, num_threads_5093);
    int32_t padded_sizze_5393 = sizze_4821 + y_5392;
    int32_t per_chunk_5395 = squot32(padded_sizze_5393, num_threads_5093);
    int64_t binop_x_5427 = sext_i32_i64(y_5392);
    int64_t bytes_5426 = 8 * binop_x_5427;
    struct memblock_device mem_5428;
    
    mem_5428.references = NULL;
    memblock_alloc_device(ctx, &mem_5428, bytes_5426, "mem_5428");
    
    int64_t binop_x_5430 = sext_i32_i64(padded_sizze_5393);
    int64_t bytes_5429 = 8 * binop_x_5430;
    struct memblock_device mem_5431;
    
    mem_5431.references = NULL;
    memblock_alloc_device(ctx, &mem_5431, bytes_5429, "mem_5431");
    
    int32_t tmp_offs_5517 = 0;
    
    if (sizze_4821 * sizeof(double) > 0) {
        OPENCL_SUCCEED(clEnqueueCopyBuffer(ctx->opencl.queue,
                                           values_mem_5425.mem, mem_5431.mem, 0,
                                           tmp_offs_5517 * 8, sizze_4821 *
                                           sizeof(double), 0, NULL, NULL));
        if (ctx->debugging)
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
    }
    tmp_offs_5517 += sizze_4821;
    if (y_5392 * sizeof(double) > 0) {
        OPENCL_SUCCEED(clEnqueueCopyBuffer(ctx->opencl.queue, mem_5428.mem,
                                           mem_5431.mem, 0, tmp_offs_5517 * 8,
                                           y_5392 * sizeof(double), 0, NULL,
                                           NULL));
        if (ctx->debugging)
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
    }
    tmp_offs_5517 += y_5392;
    memblock_unref_device(ctx, &mem_5428, "mem_5428");
    
    int32_t convop_x_5433 = num_threads_5093 * per_chunk_5395;
    int64_t binop_x_5434 = sext_i32_i64(convop_x_5433);
    int64_t bytes_5432 = 8 * binop_x_5434;
    struct memblock_device mem_5435;
    
    mem_5435.references = NULL;
    memblock_alloc_device(ctx, &mem_5435, bytes_5432, "mem_5435");
    
    int call_ret_5621 = futrts_map_transpose_opencl_f64(ctx, mem_5435, 0,
                                                        mem_5431, 0, 1,
                                                        per_chunk_5395,
                                                        num_threads_5093,
                                                        num_threads_5093 *
                                                        per_chunk_5395,
                                                        num_threads_5093 *
                                                        per_chunk_5395);
    
    assert(call_ret_5621 == 0);
    memblock_unref_device(ctx, &mem_5431, "mem_5431");
    
    int64_t binop_x_5446 = sext_i32_i64(num_groups_5092);
    int64_t bytes_5445 = 8 * binop_x_5446;
    struct memblock_device mem_5447;
    
    mem_5447.references = NULL;
    memblock_alloc_device(ctx, &mem_5447, bytes_5445, "mem_5447");
    
    struct memblock_device mem_5450;
    
    mem_5450.references = NULL;
    memblock_alloc_device(ctx, &mem_5450, bytes_5445, "mem_5450");
    
    struct memblock_device mem_5453;
    
    mem_5453.references = NULL;
    memblock_alloc_device(ctx, &mem_5453, bytes_5445, "mem_5453");
    
    int64_t binop_x_5437 = sext_i32_i64(group_sizze_5085);
    int64_t bytes_5436 = 8 * binop_x_5437;
    struct memblock_local mem_5438;
    
    mem_5438.references = NULL;
    
    struct memblock_local mem_5441;
    
    mem_5441.references = NULL;
    
    struct memblock_local mem_5444;
    
    mem_5444.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4821);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 0,
                                  bytes_5436, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 1,
                                  bytes_5436, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 2,
                                  bytes_5436, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 3,
                                  sizeof(sizze_4821), &sizze_4821));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 4,
                                  sizeof(num_threads_5093), &num_threads_5093));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 5,
                                  sizeof(per_thread_elements_5096),
                                  &per_thread_elements_5096));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 6,
                                  sizeof(per_chunk_5395), &per_chunk_5395));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 7,
                                  sizeof(mem_5435.mem), &mem_5435.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 8,
                                  sizeof(mem_5447.mem), &mem_5447.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 9,
                                  sizeof(mem_5450.mem), &mem_5450.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5103, 10,
                                  sizeof(mem_5453.mem), &mem_5453.mem));
    if (1 * (num_groups_5092 * group_sizze_5085) != 0) {
        const size_t global_work_sizze_5622[1] = {num_groups_5092 *
                     group_sizze_5085};
        const size_t local_work_sizze_5626[1] = {group_sizze_5085};
        int64_t time_start_5623 = 0, time_end_5624 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5103");
            fprintf(stderr, "%zu", global_work_sizze_5622[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5626[0]);
            fprintf(stderr, "].\n");
            time_start_5623 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5103,
                                              1, NULL, global_work_sizze_5622,
                                              local_work_sizze_5626, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5624 = get_wall_time();
            
            long time_diff_5625 = time_end_5624 - time_start_5623;
            
            ctx->chunked_reduce_kernel_5103_total_runtime += time_diff_5625;
            ctx->chunked_reduce_kernel_5103_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5103", time_diff_5625);
        }
    }
    memblock_unref_device(ctx, &mem_5435, "mem_5435");
    memblock_unref_local(ctx, &mem_5438, "mem_5438");
    memblock_unref_local(ctx, &mem_5441, "mem_5441");
    memblock_unref_local(ctx, &mem_5444, "mem_5444");
    
    struct memblock_device mem_5465;
    
    mem_5465.references = NULL;
    memblock_alloc_device(ctx, &mem_5465, 8, "mem_5465");
    
    struct memblock_device mem_5468;
    
    mem_5468.references = NULL;
    memblock_alloc_device(ctx, &mem_5468, 8, "mem_5468");
    
    struct memblock_device mem_5471;
    
    mem_5471.references = NULL;
    memblock_alloc_device(ctx, &mem_5471, 8, "mem_5471");
    
    int64_t binop_x_5455 = sext_i32_i64(max_num_groups_5087);
    int64_t bytes_5454 = 8 * binop_x_5455;
    struct memblock_local mem_5456;
    
    mem_5456.references = NULL;
    
    struct memblock_local mem_5459;
    
    mem_5459.references = NULL;
    
    struct memblock_local mem_5462;
    
    mem_5462.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5208, 0, bytes_5454,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5208, 1, bytes_5454,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5208, 2, bytes_5454,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5208, 3,
                                  sizeof(num_groups_5092), &num_groups_5092));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5208, 4,
                                  sizeof(mem_5447.mem), &mem_5447.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5208, 5,
                                  sizeof(mem_5450.mem), &mem_5450.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5208, 6,
                                  sizeof(mem_5453.mem), &mem_5453.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5208, 7,
                                  sizeof(mem_5465.mem), &mem_5465.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5208, 8,
                                  sizeof(mem_5468.mem), &mem_5468.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5208, 9,
                                  sizeof(mem_5471.mem), &mem_5471.mem));
    if (1 * max_num_groups_5087 != 0) {
        const size_t global_work_sizze_5627[1] = {max_num_groups_5087};
        const size_t local_work_sizze_5631[1] = {max_num_groups_5087};
        int64_t time_start_5628 = 0, time_end_5629 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5208");
            fprintf(stderr, "%zu", global_work_sizze_5627[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5631[0]);
            fprintf(stderr, "].\n");
            time_start_5628 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5208, 1, NULL,
                                              global_work_sizze_5627,
                                              local_work_sizze_5631, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5629 = get_wall_time();
            
            long time_diff_5630 = time_end_5629 - time_start_5628;
            
            ctx->reduce_kernel_5208_total_runtime += time_diff_5630;
            ctx->reduce_kernel_5208_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5208",
                    time_diff_5630);
        }
    }
    memblock_unref_device(ctx, &mem_5447, "mem_5447");
    memblock_unref_device(ctx, &mem_5450, "mem_5450");
    memblock_unref_device(ctx, &mem_5453, "mem_5453");
    memblock_unref_local(ctx, &mem_5456, "mem_5456");
    memblock_unref_local(ctx, &mem_5459, "mem_5459");
    memblock_unref_local(ctx, &mem_5462, "mem_5462");
    memblock_unref_device(ctx, &mem_5465, "mem_5465");
    memblock_unref_device(ctx, &mem_5468, "mem_5468");
    
    double read_res_5632;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5471.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5632, 0,
                                       NULL, NULL));
    
    double res_4825 = read_res_5632;
    
    memblock_unref_device(ctx, &mem_5471, "mem_5471");
    scalar_out_5516 = res_4825;
    *out_scalar_out_5620 = scalar_out_5516;
    memblock_unref_local(ctx, &mem_5462, "mem_5462");
    memblock_unref_local(ctx, &mem_5459, "mem_5459");
    memblock_unref_local(ctx, &mem_5456, "mem_5456");
    memblock_unref_device(ctx, &mem_5471, "mem_5471");
    memblock_unref_device(ctx, &mem_5468, "mem_5468");
    memblock_unref_device(ctx, &mem_5465, "mem_5465");
    memblock_unref_local(ctx, &mem_5444, "mem_5444");
    memblock_unref_local(ctx, &mem_5441, "mem_5441");
    memblock_unref_local(ctx, &mem_5438, "mem_5438");
    memblock_unref_device(ctx, &mem_5453, "mem_5453");
    memblock_unref_device(ctx, &mem_5450, "mem_5450");
    memblock_unref_device(ctx, &mem_5447, "mem_5447");
    memblock_unref_device(ctx, &mem_5435, "mem_5435");
    memblock_unref_device(ctx, &mem_5431, "mem_5431");
    memblock_unref_device(ctx, &mem_5428, "mem_5428");
    return 0;
}
static int futrts_stddev(struct futhark_context *ctx,
                         double *out_scalar_out_5633,
                         int64_t values_mem_sizze_5424,
                         struct memblock_device values_mem_5425,
                         int32_t sizze_4882)
{
    double scalar_out_5546;
    int32_t group_sizze_5244;
    
    group_sizze_5244 = ctx->sizes.group_sizze_5243;
    
    int32_t max_num_groups_5246;
    
    max_num_groups_5246 = ctx->sizes.max_num_groups_5245;
    
    int32_t y_5247 = group_sizze_5244 - 1;
    int32_t x_5248 = sizze_4882 + y_5247;
    int32_t w_div_group_sizze_5249 = squot32(x_5248, group_sizze_5244);
    int32_t num_groups_maybe_zzero_5250 = smin32(max_num_groups_5246,
                                                 w_div_group_sizze_5249);
    int32_t num_groups_5251 = smax32(1, num_groups_maybe_zzero_5250);
    int32_t num_threads_5252 = group_sizze_5244 * num_groups_5251;
    int32_t y_5253 = num_threads_5252 - 1;
    int32_t x_5254 = sizze_4882 + y_5253;
    int32_t per_thread_elements_5255 = squot32(x_5254, num_threads_5252);
    int32_t y_5390 = smod32(sizze_4882, num_threads_5252);
    int32_t x_5391 = num_threads_5252 - y_5390;
    int32_t y_5392 = smod32(x_5391, num_threads_5252);
    int32_t padded_sizze_5393 = sizze_4882 + y_5392;
    int32_t per_chunk_5395 = squot32(padded_sizze_5393, num_threads_5252);
    int64_t binop_x_5427 = sext_i32_i64(y_5392);
    int64_t bytes_5426 = 8 * binop_x_5427;
    struct memblock_device mem_5428;
    
    mem_5428.references = NULL;
    memblock_alloc_device(ctx, &mem_5428, bytes_5426, "mem_5428");
    
    int64_t binop_x_5430 = sext_i32_i64(padded_sizze_5393);
    int64_t bytes_5429 = 8 * binop_x_5430;
    struct memblock_device mem_5431;
    
    mem_5431.references = NULL;
    memblock_alloc_device(ctx, &mem_5431, bytes_5429, "mem_5431");
    
    int32_t tmp_offs_5547 = 0;
    
    if (sizze_4882 * sizeof(double) > 0) {
        OPENCL_SUCCEED(clEnqueueCopyBuffer(ctx->opencl.queue,
                                           values_mem_5425.mem, mem_5431.mem, 0,
                                           tmp_offs_5547 * 8, sizze_4882 *
                                           sizeof(double), 0, NULL, NULL));
        if (ctx->debugging)
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
    }
    tmp_offs_5547 += sizze_4882;
    if (y_5392 * sizeof(double) > 0) {
        OPENCL_SUCCEED(clEnqueueCopyBuffer(ctx->opencl.queue, mem_5428.mem,
                                           mem_5431.mem, 0, tmp_offs_5547 * 8,
                                           y_5392 * sizeof(double), 0, NULL,
                                           NULL));
        if (ctx->debugging)
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
    }
    tmp_offs_5547 += y_5392;
    memblock_unref_device(ctx, &mem_5428, "mem_5428");
    
    int32_t convop_x_5433 = num_threads_5252 * per_chunk_5395;
    int64_t binop_x_5434 = sext_i32_i64(convop_x_5433);
    int64_t bytes_5432 = 8 * binop_x_5434;
    struct memblock_device mem_5435;
    
    mem_5435.references = NULL;
    memblock_alloc_device(ctx, &mem_5435, bytes_5432, "mem_5435");
    
    int call_ret_5634 = futrts_map_transpose_opencl_f64(ctx, mem_5435, 0,
                                                        mem_5431, 0, 1,
                                                        per_chunk_5395,
                                                        num_threads_5252,
                                                        num_threads_5252 *
                                                        per_chunk_5395,
                                                        num_threads_5252 *
                                                        per_chunk_5395);
    
    assert(call_ret_5634 == 0);
    memblock_unref_device(ctx, &mem_5431, "mem_5431");
    
    int64_t binop_x_5446 = sext_i32_i64(num_groups_5251);
    int64_t bytes_5445 = 8 * binop_x_5446;
    struct memblock_device mem_5447;
    
    mem_5447.references = NULL;
    memblock_alloc_device(ctx, &mem_5447, bytes_5445, "mem_5447");
    
    struct memblock_device mem_5450;
    
    mem_5450.references = NULL;
    memblock_alloc_device(ctx, &mem_5450, bytes_5445, "mem_5450");
    
    struct memblock_device mem_5453;
    
    mem_5453.references = NULL;
    memblock_alloc_device(ctx, &mem_5453, bytes_5445, "mem_5453");
    
    int64_t binop_x_5437 = sext_i32_i64(group_sizze_5244);
    int64_t bytes_5436 = 8 * binop_x_5437;
    struct memblock_local mem_5438;
    
    mem_5438.references = NULL;
    
    struct memblock_local mem_5441;
    
    mem_5441.references = NULL;
    
    struct memblock_local mem_5444;
    
    mem_5444.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4882);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 0,
                                  bytes_5436, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 1,
                                  bytes_5436, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 2,
                                  bytes_5436, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 3,
                                  sizeof(sizze_4882), &sizze_4882));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 4,
                                  sizeof(num_threads_5252), &num_threads_5252));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 5,
                                  sizeof(per_thread_elements_5255),
                                  &per_thread_elements_5255));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 6,
                                  sizeof(per_chunk_5395), &per_chunk_5395));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 7,
                                  sizeof(mem_5435.mem), &mem_5435.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 8,
                                  sizeof(mem_5447.mem), &mem_5447.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 9,
                                  sizeof(mem_5450.mem), &mem_5450.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5262, 10,
                                  sizeof(mem_5453.mem), &mem_5453.mem));
    if (1 * (num_groups_5251 * group_sizze_5244) != 0) {
        const size_t global_work_sizze_5635[1] = {num_groups_5251 *
                     group_sizze_5244};
        const size_t local_work_sizze_5639[1] = {group_sizze_5244};
        int64_t time_start_5636 = 0, time_end_5637 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5262");
            fprintf(stderr, "%zu", global_work_sizze_5635[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5639[0]);
            fprintf(stderr, "].\n");
            time_start_5636 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5262,
                                              1, NULL, global_work_sizze_5635,
                                              local_work_sizze_5639, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5637 = get_wall_time();
            
            long time_diff_5638 = time_end_5637 - time_start_5636;
            
            ctx->chunked_reduce_kernel_5262_total_runtime += time_diff_5638;
            ctx->chunked_reduce_kernel_5262_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5262", time_diff_5638);
        }
    }
    memblock_unref_device(ctx, &mem_5435, "mem_5435");
    memblock_unref_local(ctx, &mem_5438, "mem_5438");
    memblock_unref_local(ctx, &mem_5441, "mem_5441");
    memblock_unref_local(ctx, &mem_5444, "mem_5444");
    
    struct memblock_device mem_5465;
    
    mem_5465.references = NULL;
    memblock_alloc_device(ctx, &mem_5465, 8, "mem_5465");
    
    struct memblock_device mem_5468;
    
    mem_5468.references = NULL;
    memblock_alloc_device(ctx, &mem_5468, 8, "mem_5468");
    
    struct memblock_device mem_5471;
    
    mem_5471.references = NULL;
    memblock_alloc_device(ctx, &mem_5471, 8, "mem_5471");
    
    int64_t binop_x_5455 = sext_i32_i64(max_num_groups_5246);
    int64_t bytes_5454 = 8 * binop_x_5455;
    struct memblock_local mem_5456;
    
    mem_5456.references = NULL;
    
    struct memblock_local mem_5459;
    
    mem_5459.references = NULL;
    
    struct memblock_local mem_5462;
    
    mem_5462.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5367, 0, bytes_5454,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5367, 1, bytes_5454,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5367, 2, bytes_5454,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5367, 3,
                                  sizeof(num_groups_5251), &num_groups_5251));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5367, 4,
                                  sizeof(mem_5447.mem), &mem_5447.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5367, 5,
                                  sizeof(mem_5450.mem), &mem_5450.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5367, 6,
                                  sizeof(mem_5453.mem), &mem_5453.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5367, 7,
                                  sizeof(mem_5465.mem), &mem_5465.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5367, 8,
                                  sizeof(mem_5468.mem), &mem_5468.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5367, 9,
                                  sizeof(mem_5471.mem), &mem_5471.mem));
    if (1 * max_num_groups_5246 != 0) {
        const size_t global_work_sizze_5640[1] = {max_num_groups_5246};
        const size_t local_work_sizze_5644[1] = {max_num_groups_5246};
        int64_t time_start_5641 = 0, time_end_5642 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5367");
            fprintf(stderr, "%zu", global_work_sizze_5640[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5644[0]);
            fprintf(stderr, "].\n");
            time_start_5641 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5367, 1, NULL,
                                              global_work_sizze_5640,
                                              local_work_sizze_5644, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5642 = get_wall_time();
            
            long time_diff_5643 = time_end_5642 - time_start_5641;
            
            ctx->reduce_kernel_5367_total_runtime += time_diff_5643;
            ctx->reduce_kernel_5367_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5367",
                    time_diff_5643);
        }
    }
    memblock_unref_device(ctx, &mem_5447, "mem_5447");
    memblock_unref_device(ctx, &mem_5450, "mem_5450");
    memblock_unref_device(ctx, &mem_5453, "mem_5453");
    memblock_unref_local(ctx, &mem_5456, "mem_5456");
    memblock_unref_local(ctx, &mem_5459, "mem_5459");
    memblock_unref_local(ctx, &mem_5462, "mem_5462");
    memblock_unref_device(ctx, &mem_5465, "mem_5465");
    memblock_unref_device(ctx, &mem_5468, "mem_5468");
    
    double read_res_5645;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5471.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5645, 0,
                                       NULL, NULL));
    
    double res_4886 = read_res_5645;
    
    memblock_unref_device(ctx, &mem_5471, "mem_5471");
    
    double res_4943;
    
    res_4943 = futrts_sqrt64(res_4886);
    scalar_out_5546 = res_4943;
    *out_scalar_out_5633 = scalar_out_5546;
    memblock_unref_local(ctx, &mem_5462, "mem_5462");
    memblock_unref_local(ctx, &mem_5459, "mem_5459");
    memblock_unref_local(ctx, &mem_5456, "mem_5456");
    memblock_unref_device(ctx, &mem_5471, "mem_5471");
    memblock_unref_device(ctx, &mem_5468, "mem_5468");
    memblock_unref_device(ctx, &mem_5465, "mem_5465");
    memblock_unref_local(ctx, &mem_5444, "mem_5444");
    memblock_unref_local(ctx, &mem_5441, "mem_5441");
    memblock_unref_local(ctx, &mem_5438, "mem_5438");
    memblock_unref_device(ctx, &mem_5453, "mem_5453");
    memblock_unref_device(ctx, &mem_5450, "mem_5450");
    memblock_unref_device(ctx, &mem_5447, "mem_5447");
    memblock_unref_device(ctx, &mem_5435, "mem_5435");
    memblock_unref_device(ctx, &mem_5431, "mem_5431");
    memblock_unref_device(ctx, &mem_5428, "mem_5428");
    return 0;
}
struct futhark_f64_1d {
    struct memblock_device mem;
    int64_t shape[1];
} ;
struct futhark_f64_1d *futhark_new_f64_1d(struct futhark_context *ctx,
                                          double *data, int dim0)
{
    struct futhark_f64_1d *arr = malloc(sizeof(struct futhark_f64_1d));
    
    if (arr == NULL)
        return NULL;
    lock_lock(&ctx->lock);
    arr->mem.references = NULL;
    memblock_alloc_device(ctx, &arr->mem, dim0 * sizeof(double), "arr->mem");
    arr->shape[0] = dim0;
    if (dim0 * sizeof(double) > 0)
        OPENCL_SUCCEED(clEnqueueWriteBuffer(ctx->opencl.queue, arr->mem.mem,
                                            CL_TRUE, 0, dim0 * sizeof(double),
                                            data + 0, 0, NULL, NULL));
    lock_unlock(&ctx->lock);
    return arr;
}
struct futhark_f64_1d *futhark_new_raw_f64_1d(struct futhark_context *ctx,
                                              cl_mem data, int offset, int dim0)
{
    struct futhark_f64_1d *arr = malloc(sizeof(struct futhark_f64_1d));
    
    if (arr == NULL)
        return NULL;
    lock_lock(&ctx->lock);
    arr->mem.references = NULL;
    memblock_alloc_device(ctx, &arr->mem, dim0 * sizeof(double), "arr->mem");
    arr->shape[0] = dim0;
    if (dim0 * sizeof(double) > 0) {
        OPENCL_SUCCEED(clEnqueueCopyBuffer(ctx->opencl.queue, data,
                                           arr->mem.mem, offset, 0, dim0 *
                                           sizeof(double), 0, NULL, NULL));
        if (ctx->debugging)
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
    }
    lock_unlock(&ctx->lock);
    return arr;
}
int futhark_free_f64_1d(struct futhark_context *ctx, struct futhark_f64_1d *arr)
{
    lock_lock(&ctx->lock);
    memblock_unref_device(ctx, &arr->mem, "arr->mem");
    lock_unlock(&ctx->lock);
    free(arr);
    return 0;
}
int futhark_values_f64_1d(struct futhark_context *ctx,
                          struct futhark_f64_1d *arr, double *data)
{
    lock_lock(&ctx->lock);
    if (arr->shape[0] * sizeof(double) > 0)
        OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, arr->mem.mem,
                                           CL_TRUE, 0, arr->shape[0] *
                                           sizeof(double), data + 0, 0, NULL,
                                           NULL));
    lock_unlock(&ctx->lock);
    return 0;
}
cl_mem futhark_values_raw_f64_1d(struct futhark_context *ctx,
                                 struct futhark_f64_1d *arr)
{
    return arr->mem.mem;
}
int64_t *futhark_shape_f64_1d(struct futhark_context *ctx,
                              struct futhark_f64_1d *arr)
{
    return arr->shape;
}
int futhark_entry_sum(struct futhark_context *ctx, double *out0, const
                      struct futhark_f64_1d *in0)
{
    int64_t col_mem_sizze_5424;
    struct memblock_device col_mem_5425;
    
    col_mem_5425.references = NULL;
    
    int32_t sizze_4805;
    double scalar_out_5480;
    
    lock_lock(&ctx->lock);
    col_mem_5425 = in0->mem;
    col_mem_sizze_5424 = in0->mem.size;
    sizze_4805 = in0->shape[0];
    
    int ret = futrts_sum(ctx, &scalar_out_5480, col_mem_sizze_5424,
                         col_mem_5425, sizze_4805);
    
    if (ret == 0) {
        *out0 = scalar_out_5480;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_entry_mean(struct futhark_context *ctx, double *out0, const
                       struct futhark_f64_1d *in0)
{
    int64_t col_mem_sizze_5424;
    struct memblock_device col_mem_5425;
    
    col_mem_5425.references = NULL;
    
    int32_t sizze_4812;
    double scalar_out_5498;
    
    lock_lock(&ctx->lock);
    col_mem_5425 = in0->mem;
    col_mem_sizze_5424 = in0->mem.size;
    sizze_4812 = in0->shape[0];
    
    int ret = futrts_mean(ctx, &scalar_out_5498, col_mem_sizze_5424,
                          col_mem_5425, sizze_4812);
    
    if (ret == 0) {
        *out0 = scalar_out_5498;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_entry_variance(struct futhark_context *ctx, double *out0, const
                           struct futhark_f64_1d *in0)
{
    int64_t values_mem_sizze_5424;
    struct memblock_device values_mem_5425;
    
    values_mem_5425.references = NULL;
    
    int32_t sizze_4821;
    double scalar_out_5516;
    
    lock_lock(&ctx->lock);
    values_mem_5425 = in0->mem;
    values_mem_sizze_5424 = in0->mem.size;
    sizze_4821 = in0->shape[0];
    
    int ret = futrts_variance(ctx, &scalar_out_5516, values_mem_sizze_5424,
                              values_mem_5425, sizze_4821);
    
    if (ret == 0) {
        *out0 = scalar_out_5516;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_entry_stddev(struct futhark_context *ctx, double *out0, const
                         struct futhark_f64_1d *in0)
{
    int64_t values_mem_sizze_5424;
    struct memblock_device values_mem_5425;
    
    values_mem_5425.references = NULL;
    
    int32_t sizze_4882;
    double scalar_out_5546;
    
    lock_lock(&ctx->lock);
    values_mem_5425 = in0->mem;
    values_mem_sizze_5424 = in0->mem.size;
    sizze_4882 = in0->shape[0];
    
    int ret = futrts_stddev(ctx, &scalar_out_5546, values_mem_sizze_5424,
                            values_mem_5425, sizze_4882);
    
    if (ret == 0) {
        *out0 = scalar_out_5546;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
