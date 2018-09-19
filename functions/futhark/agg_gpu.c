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
            "_sin64(double x)\n{\n    return sin(x);\n}\nstatic inline double futrts_tan64(double x)\n{\n    return tan(x);\n}\nstatic inline double futrts_acos64(double x)\n{\n    return acos(x);\n}\nstatic inline double futrts_asin64(double x)\n{\n    return asin(x);\n}\nstatic inline double futrts_atan64(double x)\n{\n    return atan(x);\n}\nstatic inline double futrts_atan2_64(double x, double y)\n{\n    return atan2(x, y);\n}\nstatic inline double futrts_round64(double x)\n{\n    return rint(x);\n}\nstatic inline char futrts_isnan64(double x)\n{\n    return isnan(x);\n}\nstatic inline char futrts_isinf64(double x)\n{\n    return isinf(x);\n}\nstatic inline int64_t futrts_to_bits64(double x)\n{\n    union {\n        double f;\n        int64_t t;\n    } p;\n    \n    p.f = x;\n    return p.t;\n}\nstatic inline double futrts_from_bits64(int64_t x)\n{\n    union {\n        int64_t f;\n        double t;\n    } p;\n    \n    p.f = x;\n    return p.t;\n}\nstatic inline float fpconv_f32_f32(float x)\n{\n    return x;\n}\nstatic inline double fpconv_f32_f64(float x)\n{\n    return x;\n}\nstatic inline float fpconv_f64_f32(double x)\n{\n    return x;\n}\nstatic inline double fpconv_f64_f64(double x)\n{\n    return x;\n}\n#define group_sizze_4954 (group_size_4953)\n#define max_num_groups_4956 (max_num_groups_4955)\n#define group_sizze_5015 (group_size_5014)\n#define max_num_groups_5017 (max_num_groups_5016)\n#define group_sizze_5076 (group_size_5075)\n#define max_num_groups_5078 (max_num_groups_5077)\n#define group_sizze_5137 (group_size_5136)\n#define max_num_groups_5139 (max_num_groups_5138)\n#define group_sizze_5200 (group_size_5199)\n#define max_num_groups_5202 (max_num_groups_5201)\n#define group_sizze_5263 (group_size_5262)\n#define max_num_groups_5265 (max_num_groups_5264)\n#define group_sizze_5349 (group_size_5348)\n#define max_num_groups_5351 (max_num_groups_5350)\n#define group_sizze_5412 (group_size_5411)\n#define max_num_groups_5414 (max_num_groups_5413)\n#define group_sizze_5499 (group_size_5498)\n#define max_num_groups_5501 (max_num_groups_5500)\n#define grou",
            "p_sizze_5560 (group_size_5559)\n#define max_num_groups_5562 (max_num_groups_5561)\n__kernel void chunked_reduce_kernel_4970(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         int32_t sizze_4841,\n                                         int32_t num_threads_4962,\n                                         int32_t per_thread_elements_4965,\n                                         __global unsigned char *col_mem_5630,\n                                         __global unsigned char *mem_5636)\n{\n    __local volatile char *restrict mem_5633 = mem_aligned_0;\n    int32_t wave_sizze_5688;\n    int32_t group_sizze_5689;\n    bool thread_active_5690;\n    int32_t global_tid_4970;\n    int32_t local_tid_4971;\n    int32_t group_id_4972;\n    \n    global_tid_4970 = get_global_id(0);\n    local_tid_4971 = get_local_id(0);\n    group_sizze_5689 = get_local_size(0);\n    wave_sizze_5688 = LOCKSTEP_WIDTH;\n    group_id_4972 = get_group_id(0);\n    thread_active_5690 = 1;\n    \n    int32_t chunk_sizze_4977 = smin32(per_thread_elements_4965,\n                                      squot32(sizze_4841 - global_tid_4970 +\n                                              num_threads_4962 - 1,\n                                              num_threads_4962));\n    double res_4980;\n    \n    if (thread_active_5690) {\n        double acc_4983 = 0.0;\n        \n        for (int32_t i_4982 = 0; i_4982 < chunk_sizze_4977; i_4982++) {\n            int32_t j_t_s_5623 = num_threads_4962 * i_4982;\n            int32_t j_p_i_t_s_5624 = global_tid_4970 + j_t_s_5623;\n            double x_4985 = *(__global double *) &col_mem_5630[j_p_i_t_s_5624 *\n                                                               8];\n            double res_4988 = acc_4983 + x_4985;\n            double acc_tmp_5691 = res_4988;\n            \n            acc_4983 = acc_tmp_5691;\n        }\n        res_4980 = acc_4983;\n    }\n    \n    double final_result_4991;\n    \n    for (int32_t comb_ite",
            "r_5692 = 0; comb_iter_5692 < squot32(group_sizze_4954 +\n                                                              group_sizze_4954 -\n                                                              1,\n                                                              group_sizze_4954);\n         comb_iter_5692++) {\n        int32_t combine_id_4975;\n        int32_t flat_comb_id_5693 = comb_iter_5692 * group_sizze_4954 +\n                local_tid_4971;\n        \n        combine_id_4975 = flat_comb_id_5693;\n        if (slt32(combine_id_4975, group_sizze_4954) && 1) {\n            *(__local double *) &mem_5633[combine_id_4975 * 8] = res_4980;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5695;\n    int32_t skip_waves_5694;\n    int32_t my_index_4992;\n    int32_t other_index_4993;\n    double x_4994;\n    double x_4995;\n    \n    my_index_4992 = local_tid_4971;\n    offset_5695 = 0;\n    other_index_4993 = local_tid_4971 + offset_5695;\n    if (slt32(local_tid_4971, group_sizze_4954)) {\n        x_4994 = *(__local double *) &mem_5633[(local_tid_4971 + offset_5695) *\n                                               8];\n    }\n    offset_5695 = 1;\n    other_index_4993 = local_tid_4971 + offset_5695;\n    while (slt32(offset_5695, wave_sizze_5688)) {\n        if (slt32(other_index_4993, group_sizze_4954) && ((local_tid_4971 -\n                                                           squot32(local_tid_4971,\n                                                                   wave_sizze_5688) *\n                                                           wave_sizze_5688) &\n                                                          (2 * offset_5695 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_4995 = *(volatile __local\n                           double *) &mem_5633[(local_tid_4971 + offset_5695) *\n                                               8];\n            }\n            \n         ",
            "   double res_4996;\n            \n            if (thread_active_5690) {\n                res_4996 = x_4994 + x_4995;\n            }\n            x_4994 = res_4996;\n            *(volatile __local double *) &mem_5633[local_tid_4971 * 8] = x_4994;\n        }\n        offset_5695 *= 2;\n        other_index_4993 = local_tid_4971 + offset_5695;\n    }\n    skip_waves_5694 = 1;\n    while (slt32(skip_waves_5694, squot32(group_sizze_4954 + wave_sizze_5688 -\n                                          1, wave_sizze_5688))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5695 = skip_waves_5694 * wave_sizze_5688;\n        other_index_4993 = local_tid_4971 + offset_5695;\n        if (slt32(other_index_4993, group_sizze_4954) && ((local_tid_4971 -\n                                                           squot32(local_tid_4971,\n                                                                   wave_sizze_5688) *\n                                                           wave_sizze_5688) ==\n                                                          0 &&\n                                                          (squot32(local_tid_4971,\n                                                                   wave_sizze_5688) &\n                                                           (2 *\n                                                            skip_waves_5694 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_4995 = *(__local double *) &mem_5633[(local_tid_4971 +\n                                                        offset_5695) * 8];\n            }\n            \n            double res_4996;\n            \n            if (thread_active_5690) {\n                res_4996 = x_4994 + x_4995;\n            }\n            x_4994 = res_4996;\n            *(__local double *) &mem_5633[local_tid_4971 * 8] = x_4994;\n        }\n        skip_waves_5694 *= 2;\n    }\n    final_result_4991 = x_4994;\n    if (local_tid_4971 ==",
            " 0) {\n        *(__global double *) &mem_5636[group_id_4972 * 8] = final_result_4991;\n    }\n}\n__kernel void chunked_reduce_kernel_5031(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         int32_t sizze_4848,\n                                         int32_t num_threads_5023,\n                                         int32_t per_thread_elements_5026,\n                                         __global unsigned char *col_mem_5630,\n                                         __global unsigned char *mem_5636)\n{\n    __local volatile char *restrict mem_5633 = mem_aligned_0;\n    int32_t wave_sizze_5706;\n    int32_t group_sizze_5707;\n    bool thread_active_5708;\n    int32_t global_tid_5031;\n    int32_t local_tid_5032;\n    int32_t group_id_5033;\n    \n    global_tid_5031 = get_global_id(0);\n    local_tid_5032 = get_local_id(0);\n    group_sizze_5707 = get_local_size(0);\n    wave_sizze_5706 = LOCKSTEP_WIDTH;\n    group_id_5033 = get_group_id(0);\n    thread_active_5708 = 1;\n    \n    int32_t chunk_sizze_5038 = smin32(per_thread_elements_5026,\n                                      squot32(sizze_4848 - global_tid_5031 +\n                                              num_threads_5023 - 1,\n                                              num_threads_5023));\n    double res_5041;\n    \n    if (thread_active_5708) {\n        double acc_5044 = 0.0;\n        \n        for (int32_t i_5043 = 0; i_5043 < chunk_sizze_5038; i_5043++) {\n            int32_t j_t_s_5623 = num_threads_5023 * i_5043;\n            int32_t j_p_i_t_s_5624 = global_tid_5031 + j_t_s_5623;\n            double x_5046 = *(__global double *) &col_mem_5630[j_p_i_t_s_5624 *\n                                                               8];\n            double res_5049 = acc_5044 + x_5046;\n            double acc_tmp_5709 = res_5049;\n            \n            acc_5044 = acc_tmp_5709;\n        }\n        res_5041 = acc_5044;\n    }\n    \n    double final_result_5052;\n    \n    for (int3",
            "2_t comb_iter_5710 = 0; comb_iter_5710 < squot32(group_sizze_5015 +\n                                                              group_sizze_5015 -\n                                                              1,\n                                                              group_sizze_5015);\n         comb_iter_5710++) {\n        int32_t combine_id_5036;\n        int32_t flat_comb_id_5711 = comb_iter_5710 * group_sizze_5015 +\n                local_tid_5032;\n        \n        combine_id_5036 = flat_comb_id_5711;\n        if (slt32(combine_id_5036, group_sizze_5015) && 1) {\n            *(__local double *) &mem_5633[combine_id_5036 * 8] = res_5041;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5713;\n    int32_t skip_waves_5712;\n    int32_t my_index_5053;\n    int32_t other_index_5054;\n    double x_5055;\n    double x_5056;\n    \n    my_index_5053 = local_tid_5032;\n    offset_5713 = 0;\n    other_index_5054 = local_tid_5032 + offset_5713;\n    if (slt32(local_tid_5032, group_sizze_5015)) {\n        x_5055 = *(__local double *) &mem_5633[(local_tid_5032 + offset_5713) *\n                                               8];\n    }\n    offset_5713 = 1;\n    other_index_5054 = local_tid_5032 + offset_5713;\n    while (slt32(offset_5713, wave_sizze_5706)) {\n        if (slt32(other_index_5054, group_sizze_5015) && ((local_tid_5032 -\n                                                           squot32(local_tid_5032,\n                                                                   wave_sizze_5706) *\n                                                           wave_sizze_5706) &\n                                                          (2 * offset_5713 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5056 = *(volatile __local\n                           double *) &mem_5633[(local_tid_5032 + offset_5713) *\n                                               8];\n            }\n          ",
            "  \n            double res_5057;\n            \n            if (thread_active_5708) {\n                res_5057 = x_5055 + x_5056;\n            }\n            x_5055 = res_5057;\n            *(volatile __local double *) &mem_5633[local_tid_5032 * 8] = x_5055;\n        }\n        offset_5713 *= 2;\n        other_index_5054 = local_tid_5032 + offset_5713;\n    }\n    skip_waves_5712 = 1;\n    while (slt32(skip_waves_5712, squot32(group_sizze_5015 + wave_sizze_5706 -\n                                          1, wave_sizze_5706))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5713 = skip_waves_5712 * wave_sizze_5706;\n        other_index_5054 = local_tid_5032 + offset_5713;\n        if (slt32(other_index_5054, group_sizze_5015) && ((local_tid_5032 -\n                                                           squot32(local_tid_5032,\n                                                                   wave_sizze_5706) *\n                                                           wave_sizze_5706) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5032,\n                                                                   wave_sizze_5706) &\n                                                           (2 *\n                                                            skip_waves_5712 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5056 = *(__local double *) &mem_5633[(local_tid_5032 +\n                                                        offset_5713) * 8];\n            }\n            \n            double res_5057;\n            \n            if (thread_active_5708) {\n                res_5057 = x_5055 + x_5056;\n            }\n            x_5055 = res_5057;\n            *(__local double *) &mem_5633[local_tid_5032 * 8] = x_5055;\n        }\n        skip_waves_5712 *= 2;\n    }\n    final_result_5052 = x_5055;\n    if (local",
            "_tid_5032 == 0) {\n        *(__global double *) &mem_5636[group_id_5033 * 8] = final_result_5052;\n    }\n}\n__kernel void chunked_reduce_kernel_5092(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         int32_t sizze_4857,\n                                         int32_t num_threads_5084,\n                                         int32_t per_thread_elements_5087,\n                                         __global\n                                         unsigned char *values_mem_5630,\n                                         __global unsigned char *mem_5636)\n{\n    __local volatile char *restrict mem_5633 = mem_aligned_0;\n    int32_t wave_sizze_5724;\n    int32_t group_sizze_5725;\n    bool thread_active_5726;\n    int32_t global_tid_5092;\n    int32_t local_tid_5093;\n    int32_t group_id_5094;\n    \n    global_tid_5092 = get_global_id(0);\n    local_tid_5093 = get_local_id(0);\n    group_sizze_5725 = get_local_size(0);\n    wave_sizze_5724 = LOCKSTEP_WIDTH;\n    group_id_5094 = get_group_id(0);\n    thread_active_5726 = 1;\n    \n    int32_t chunk_sizze_5099 = smin32(per_thread_elements_5087,\n                                      squot32(sizze_4857 - global_tid_5092 +\n                                              num_threads_5084 - 1,\n                                              num_threads_5084));\n    double res_5102;\n    \n    if (thread_active_5726) {\n        double acc_5105 = 0.0;\n        \n        for (int32_t i_5104 = 0; i_5104 < chunk_sizze_5099; i_5104++) {\n            int32_t j_t_s_5623 = num_threads_5084 * i_5104;\n            int32_t j_p_i_t_s_5624 = global_tid_5092 + j_t_s_5623;\n            double x_5107 = *(__global\n                              double *) &values_mem_5630[j_p_i_t_s_5624 * 8];\n            double res_5110 = acc_5105 + x_5107;\n            double acc_tmp_5727 = res_5110;\n            \n            acc_5105 = acc_tmp_5727;\n        }\n        res_5102 = acc_5105;\n    }\n    \n    double final_resul",
            "t_5113;\n    \n    for (int32_t comb_iter_5728 = 0; comb_iter_5728 < squot32(group_sizze_5076 +\n                                                              group_sizze_5076 -\n                                                              1,\n                                                              group_sizze_5076);\n         comb_iter_5728++) {\n        int32_t combine_id_5097;\n        int32_t flat_comb_id_5729 = comb_iter_5728 * group_sizze_5076 +\n                local_tid_5093;\n        \n        combine_id_5097 = flat_comb_id_5729;\n        if (slt32(combine_id_5097, group_sizze_5076) && 1) {\n            *(__local double *) &mem_5633[combine_id_5097 * 8] = res_5102;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5731;\n    int32_t skip_waves_5730;\n    int32_t my_index_5114;\n    int32_t other_index_5115;\n    double x_5116;\n    double x_5117;\n    \n    my_index_5114 = local_tid_5093;\n    offset_5731 = 0;\n    other_index_5115 = local_tid_5093 + offset_5731;\n    if (slt32(local_tid_5093, group_sizze_5076)) {\n        x_5116 = *(__local double *) &mem_5633[(local_tid_5093 + offset_5731) *\n                                               8];\n    }\n    offset_5731 = 1;\n    other_index_5115 = local_tid_5093 + offset_5731;\n    while (slt32(offset_5731, wave_sizze_5724)) {\n        if (slt32(other_index_5115, group_sizze_5076) && ((local_tid_5093 -\n                                                           squot32(local_tid_5093,\n                                                                   wave_sizze_5724) *\n                                                           wave_sizze_5724) &\n                                                          (2 * offset_5731 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5117 = *(volatile __local\n                           double *) &mem_5633[(local_tid_5093 + offset_5731) *\n                                               8]",
            ";\n            }\n            \n            double res_5118;\n            \n            if (thread_active_5726) {\n                res_5118 = x_5116 + x_5117;\n            }\n            x_5116 = res_5118;\n            *(volatile __local double *) &mem_5633[local_tid_5093 * 8] = x_5116;\n        }\n        offset_5731 *= 2;\n        other_index_5115 = local_tid_5093 + offset_5731;\n    }\n    skip_waves_5730 = 1;\n    while (slt32(skip_waves_5730, squot32(group_sizze_5076 + wave_sizze_5724 -\n                                          1, wave_sizze_5724))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5731 = skip_waves_5730 * wave_sizze_5724;\n        other_index_5115 = local_tid_5093 + offset_5731;\n        if (slt32(other_index_5115, group_sizze_5076) && ((local_tid_5093 -\n                                                           squot32(local_tid_5093,\n                                                                   wave_sizze_5724) *\n                                                           wave_sizze_5724) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5093,\n                                                                   wave_sizze_5724) &\n                                                           (2 *\n                                                            skip_waves_5730 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5117 = *(__local double *) &mem_5633[(local_tid_5093 +\n                                                        offset_5731) * 8];\n            }\n            \n            double res_5118;\n            \n            if (thread_active_5726) {\n                res_5118 = x_5116 + x_5117;\n            }\n            x_5116 = res_5118;\n            *(__local double *) &mem_5633[local_tid_5093 * 8] = x_5116;\n        }\n        skip_waves_5730 *= 2;\n    }\n    final_result_51",
            "13 = x_5116;\n    if (local_tid_5093 == 0) {\n        *(__global double *) &mem_5636[group_id_5094 * 8] = final_result_5113;\n    }\n}\n__kernel void chunked_reduce_kernel_5153(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         int32_t sizze_4857, double res_4865,\n                                         int32_t num_threads_5145,\n                                         int32_t per_thread_elements_5148,\n                                         __global\n                                         unsigned char *values_mem_5630,\n                                         __global unsigned char *mem_5648)\n{\n    __local volatile char *restrict mem_5645 = mem_aligned_0;\n    int32_t wave_sizze_5741;\n    int32_t group_sizze_5742;\n    bool thread_active_5743;\n    int32_t global_tid_5153;\n    int32_t local_tid_5154;\n    int32_t group_id_5155;\n    \n    global_tid_5153 = get_global_id(0);\n    local_tid_5154 = get_local_id(0);\n    group_sizze_5742 = get_local_size(0);\n    wave_sizze_5741 = LOCKSTEP_WIDTH;\n    group_id_5155 = get_group_id(0);\n    thread_active_5743 = 1;\n    \n    int32_t chunk_sizze_5160 = smin32(per_thread_elements_5148,\n                                      squot32(sizze_4857 - global_tid_5153 +\n                                              num_threads_5145 - 1,\n                                              num_threads_5145));\n    double res_5163;\n    \n    if (thread_active_5743) {\n        double acc_5166 = 0.0;\n        \n        for (int32_t i_5165 = 0; i_5165 < chunk_sizze_5160; i_5165++) {\n            int32_t j_t_s_5627 = num_threads_5145 * i_5165;\n            int32_t j_p_i_t_s_5628 = global_tid_5153 + j_t_s_5627;\n            double x_5168 = *(__global\n                              double *) &values_mem_5630[j_p_i_t_s_5628 * 8];\n            double res_5170 = x_5168 - res_4865;\n            double res_5171 = res_5170 * res_5170;\n            double res_5173 = acc_5166 + res_5171;\n            double ac",
            "c_tmp_5744 = res_5173;\n            \n            acc_5166 = acc_tmp_5744;\n        }\n        res_5163 = acc_5166;\n    }\n    \n    double final_result_5176;\n    \n    for (int32_t comb_iter_5745 = 0; comb_iter_5745 < squot32(group_sizze_5137 +\n                                                              group_sizze_5137 -\n                                                              1,\n                                                              group_sizze_5137);\n         comb_iter_5745++) {\n        int32_t combine_id_5158;\n        int32_t flat_comb_id_5746 = comb_iter_5745 * group_sizze_5137 +\n                local_tid_5154;\n        \n        combine_id_5158 = flat_comb_id_5746;\n        if (slt32(combine_id_5158, group_sizze_5137) && 1) {\n            *(__local double *) &mem_5645[combine_id_5158 * 8] = res_5163;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5748;\n    int32_t skip_waves_5747;\n    int32_t my_index_5177;\n    int32_t other_index_5178;\n    double x_5179;\n    double x_5180;\n    \n    my_index_5177 = local_tid_5154;\n    offset_5748 = 0;\n    other_index_5178 = local_tid_5154 + offset_5748;\n    if (slt32(local_tid_5154, group_sizze_5137)) {\n        x_5179 = *(__local double *) &mem_5645[(local_tid_5154 + offset_5748) *\n                                               8];\n    }\n    offset_5748 = 1;\n    other_index_5178 = local_tid_5154 + offset_5748;\n    while (slt32(offset_5748, wave_sizze_5741)) {\n        if (slt32(other_index_5178, group_sizze_5137) && ((local_tid_5154 -\n                                                           squot32(local_tid_5154,\n                                                                   wave_sizze_5741) *\n                                                           wave_sizze_5741) &\n                                                          (2 * offset_5748 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5180 = *(v",
            "olatile __local\n                           double *) &mem_5645[(local_tid_5154 + offset_5748) *\n                                               8];\n            }\n            \n            double res_5181;\n            \n            if (thread_active_5743) {\n                res_5181 = x_5179 + x_5180;\n            }\n            x_5179 = res_5181;\n            *(volatile __local double *) &mem_5645[local_tid_5154 * 8] = x_5179;\n        }\n        offset_5748 *= 2;\n        other_index_5178 = local_tid_5154 + offset_5748;\n    }\n    skip_waves_5747 = 1;\n    while (slt32(skip_waves_5747, squot32(group_sizze_5137 + wave_sizze_5741 -\n                                          1, wave_sizze_5741))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5748 = skip_waves_5747 * wave_sizze_5741;\n        other_index_5178 = local_tid_5154 + offset_5748;\n        if (slt32(other_index_5178, group_sizze_5137) && ((local_tid_5154 -\n                                                           squot32(local_tid_5154,\n                                                                   wave_sizze_5741) *\n                                                           wave_sizze_5741) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5154,\n                                                                   wave_sizze_5741) &\n                                                           (2 *\n                                                            skip_waves_5747 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5180 = *(__local double *) &mem_5645[(local_tid_5154 +\n                                                        offset_5748) * 8];\n            }\n            \n            double res_5181;\n            \n            if (thread_active_5743) {\n                res_5181 = x_5179 + x_5180;\n            }\n            x_5179 = re",
            "s_5181;\n            *(__local double *) &mem_5645[local_tid_5154 * 8] = x_5179;\n        }\n        skip_waves_5747 *= 2;\n    }\n    final_result_5176 = x_5179;\n    if (local_tid_5154 == 0) {\n        *(__global double *) &mem_5648[group_id_5155 * 8] = final_result_5176;\n    }\n}\n__kernel void chunked_reduce_kernel_5216(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         int32_t sizze_4875,\n                                         int32_t num_threads_5208,\n                                         int32_t per_thread_elements_5211,\n                                         __global\n                                         unsigned char *values_mem_5630,\n                                         __global unsigned char *mem_5636)\n{\n    __local volatile char *restrict mem_5633 = mem_aligned_0;\n    int32_t wave_sizze_5759;\n    int32_t group_sizze_5760;\n    bool thread_active_5761;\n    int32_t global_tid_5216;\n    int32_t local_tid_5217;\n    int32_t group_id_5218;\n    \n    global_tid_5216 = get_global_id(0);\n    local_tid_5217 = get_local_id(0);\n    group_sizze_5760 = get_local_size(0);\n    wave_sizze_5759 = LOCKSTEP_WIDTH;\n    group_id_5218 = get_group_id(0);\n    thread_active_5761 = 1;\n    \n    int32_t chunk_sizze_5223 = smin32(per_thread_elements_5211,\n                                      squot32(sizze_4875 - global_tid_5216 +\n                                              num_threads_5208 - 1,\n                                              num_threads_5208));\n    double res_5226;\n    \n    if (thread_active_5761) {\n        double acc_5229 = 0.0;\n        \n        for (int32_t i_5228 = 0; i_5228 < chunk_sizze_5223; i_5228++) {\n            int32_t j_t_s_5623 = num_threads_5208 * i_5228;\n            int32_t j_p_i_t_s_5624 = global_tid_5216 + j_t_s_5623;\n            double x_5231 = *(__global\n                              double *) &values_mem_5630[j_p_i_t_s_5624 * 8];\n            double res_5234 = acc_5229 + x_5",
            "231;\n            double acc_tmp_5762 = res_5234;\n            \n            acc_5229 = acc_tmp_5762;\n        }\n        res_5226 = acc_5229;\n    }\n    \n    double final_result_5237;\n    \n    for (int32_t comb_iter_5763 = 0; comb_iter_5763 < squot32(group_sizze_5200 +\n                                                              group_sizze_5200 -\n                                                              1,\n                                                              group_sizze_5200);\n         comb_iter_5763++) {\n        int32_t combine_id_5221;\n        int32_t flat_comb_id_5764 = comb_iter_5763 * group_sizze_5200 +\n                local_tid_5217;\n        \n        combine_id_5221 = flat_comb_id_5764;\n        if (slt32(combine_id_5221, group_sizze_5200) && 1) {\n            *(__local double *) &mem_5633[combine_id_5221 * 8] = res_5226;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5766;\n    int32_t skip_waves_5765;\n    int32_t my_index_5238;\n    int32_t other_index_5239;\n    double x_5240;\n    double x_5241;\n    \n    my_index_5238 = local_tid_5217;\n    offset_5766 = 0;\n    other_index_5239 = local_tid_5217 + offset_5766;\n    if (slt32(local_tid_5217, group_sizze_5200)) {\n        x_5240 = *(__local double *) &mem_5633[(local_tid_5217 + offset_5766) *\n                                               8];\n    }\n    offset_5766 = 1;\n    other_index_5239 = local_tid_5217 + offset_5766;\n    while (slt32(offset_5766, wave_sizze_5759)) {\n        if (slt32(other_index_5239, group_sizze_5200) && ((local_tid_5217 -\n                                                           squot32(local_tid_5217,\n                                                                   wave_sizze_5759) *\n                                                           wave_sizze_5759) &\n                                                          (2 * offset_5766 -\n                                                           1)) == 0) {\n            // read array element\n            {\n  ",
            "              x_5241 = *(volatile __local\n                           double *) &mem_5633[(local_tid_5217 + offset_5766) *\n                                               8];\n            }\n            \n            double res_5242;\n            \n            if (thread_active_5761) {\n                res_5242 = x_5240 + x_5241;\n            }\n            x_5240 = res_5242;\n            *(volatile __local double *) &mem_5633[local_tid_5217 * 8] = x_5240;\n        }\n        offset_5766 *= 2;\n        other_index_5239 = local_tid_5217 + offset_5766;\n    }\n    skip_waves_5765 = 1;\n    while (slt32(skip_waves_5765, squot32(group_sizze_5200 + wave_sizze_5759 -\n                                          1, wave_sizze_5759))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5766 = skip_waves_5765 * wave_sizze_5759;\n        other_index_5239 = local_tid_5217 + offset_5766;\n        if (slt32(other_index_5239, group_sizze_5200) && ((local_tid_5217 -\n                                                           squot32(local_tid_5217,\n                                                                   wave_sizze_5759) *\n                                                           wave_sizze_5759) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5217,\n                                                                   wave_sizze_5759) &\n                                                           (2 *\n                                                            skip_waves_5765 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5241 = *(__local double *) &mem_5633[(local_tid_5217 +\n                                                        offset_5766) * 8];\n            }\n            \n            double res_5242;\n            \n            if (thread_active_5761) {\n                res_5242 = x_5240 + x_5241;\n           ",
            " }\n            x_5240 = res_5242;\n            *(__local double *) &mem_5633[local_tid_5217 * 8] = x_5240;\n        }\n        skip_waves_5765 *= 2;\n    }\n    final_result_5237 = x_5240;\n    if (local_tid_5217 == 0) {\n        *(__global double *) &mem_5636[group_id_5218 * 8] = final_result_5237;\n    }\n}\n__kernel void chunked_reduce_kernel_5280(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         __local volatile\n                                         int64_t *mem_aligned_1,\n                                         int32_t sizze_4875, double res_4883,\n                                         int32_t num_threads_5271,\n                                         int32_t per_thread_elements_5274,\n                                         __global\n                                         unsigned char *values_mem_5630,\n                                         __global unsigned char *mem_5651,\n                                         __global unsigned char *mem_5654)\n{\n    __local volatile char *restrict mem_5645 = mem_aligned_0;\n    __local volatile char *restrict mem_5648 = mem_aligned_1;\n    int32_t wave_sizze_5776;\n    int32_t group_sizze_5777;\n    bool thread_active_5778;\n    int32_t global_tid_5280;\n    int32_t local_tid_5281;\n    int32_t group_id_5282;\n    \n    global_tid_5280 = get_global_id(0);\n    local_tid_5281 = get_local_id(0);\n    group_sizze_5777 = get_local_size(0);\n    wave_sizze_5776 = LOCKSTEP_WIDTH;\n    group_id_5282 = get_group_id(0);\n    thread_active_5778 = 1;\n    \n    int32_t chunk_sizze_5291 = smin32(per_thread_elements_5274,\n                                      squot32(sizze_4875 - global_tid_5280 +\n                                              num_threads_5271 - 1,\n                                              num_threads_5271));\n    double res_5295;\n    double res_5296;\n    \n    if (thread_active_5778) {\n        double acc_5299;\n        double acc_5300;\n        \n        acc_5299 ",
            "= 0.0;\n        acc_5300 = 0.0;\n        for (int32_t i_5298 = 0; i_5298 < chunk_sizze_5291; i_5298++) {\n            int32_t j_t_s_5627 = num_threads_5271 * i_5298;\n            int32_t j_p_i_t_s_5628 = global_tid_5280 + j_t_s_5627;\n            double x_5302 = *(__global\n                              double *) &values_mem_5630[j_p_i_t_s_5628 * 8];\n            double res_5305 = x_5302 - res_4883;\n            double res_5306 = res_5305 * res_5305;\n            double res_5307 = res_5305 * res_5306;\n            double res_5310 = acc_5299 + res_5306;\n            double res_5311 = acc_5300 + res_5307;\n            double acc_tmp_5779 = res_5310;\n            double acc_tmp_5780;\n            \n            acc_tmp_5780 = res_5311;\n            acc_5299 = acc_tmp_5779;\n            acc_5300 = acc_tmp_5780;\n        }\n        res_5295 = acc_5299;\n        res_5296 = acc_5300;\n    }\n    \n    double final_result_5316;\n    double final_result_5317;\n    \n    for (int32_t comb_iter_5781 = 0; comb_iter_5781 < squot32(group_sizze_5263 +\n                                                              group_sizze_5263 -\n                                                              1,\n                                                              group_sizze_5263);\n         comb_iter_5781++) {\n        int32_t combine_id_5287;\n        int32_t flat_comb_id_5782 = comb_iter_5781 * group_sizze_5263 +\n                local_tid_5281;\n        \n        combine_id_5287 = flat_comb_id_5782;\n        if (slt32(combine_id_5287, group_sizze_5263) && 1) {\n            *(__local double *) &mem_5645[combine_id_5287 * 8] = res_5295;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    for (int32_t comb_iter_5783 = 0; comb_iter_5783 < squot32(group_sizze_5263 +\n                                                              group_sizze_5263 -\n                                                              1,\n                                                              group_sizze_5263);\n         comb_iter_5783++) {\n    ",
            "    int32_t combine_id_5288;\n        int32_t flat_comb_id_5784 = comb_iter_5783 * group_sizze_5263 +\n                local_tid_5281;\n        \n        combine_id_5288 = flat_comb_id_5784;\n        if (slt32(combine_id_5288, group_sizze_5263) && 1) {\n            *(__local double *) &mem_5648[combine_id_5288 * 8] = res_5296;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5786;\n    int32_t skip_waves_5785;\n    int32_t my_index_5318;\n    int32_t other_index_5319;\n    double x_5320;\n    double x_5321;\n    double x_5322;\n    double x_5323;\n    \n    my_index_5318 = local_tid_5281;\n    offset_5786 = 0;\n    other_index_5319 = local_tid_5281 + offset_5786;\n    if (slt32(local_tid_5281, group_sizze_5263)) {\n        x_5320 = *(__local double *) &mem_5645[(local_tid_5281 + offset_5786) *\n                                               8];\n        x_5321 = *(__local double *) &mem_5648[(local_tid_5281 + offset_5786) *\n                                               8];\n    }\n    offset_5786 = 1;\n    other_index_5319 = local_tid_5281 + offset_5786;\n    while (slt32(offset_5786, wave_sizze_5776)) {\n        if (slt32(other_index_5319, group_sizze_5263) && ((local_tid_5281 -\n                                                           squot32(local_tid_5281,\n                                                                   wave_sizze_5776) *\n                                                           wave_sizze_5776) &\n                                                          (2 * offset_5786 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5322 = *(volatile __local\n                           double *) &mem_5645[(local_tid_5281 + offset_5786) *\n                                               8];\n                x_5323 = *(volatile __local\n                           double *) &mem_5648[(local_tid_5281 + offset_5786) *\n                                               8];\n        ",
            "    }\n            \n            double res_5324;\n            double res_5325;\n            \n            if (thread_active_5778) {\n                res_5324 = x_5320 + x_5322;\n                res_5325 = x_5321 + x_5323;\n            }\n            x_5320 = res_5324;\n            x_5321 = res_5325;\n            *(volatile __local double *) &mem_5645[local_tid_5281 * 8] = x_5320;\n            *(volatile __local double *) &mem_5648[local_tid_5281 * 8] = x_5321;\n        }\n        offset_5786 *= 2;\n        other_index_5319 = local_tid_5281 + offset_5786;\n    }\n    skip_waves_5785 = 1;\n    while (slt32(skip_waves_5785, squot32(group_sizze_5263 + wave_sizze_5776 -\n                                          1, wave_sizze_5776))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5786 = skip_waves_5785 * wave_sizze_5776;\n        other_index_5319 = local_tid_5281 + offset_5786;\n        if (slt32(other_index_5319, group_sizze_5263) && ((local_tid_5281 -\n                                                           squot32(local_tid_5281,\n                                                                   wave_sizze_5776) *\n                                                           wave_sizze_5776) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5281,\n                                                                   wave_sizze_5776) &\n                                                           (2 *\n                                                            skip_waves_5785 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5322 = *(__local double *) &mem_5645[(local_tid_5281 +\n                                                        offset_5786) * 8];\n                x_5323 = *(__local double *) &mem_5648[(local_tid_5281 +\n                                                        offset_5786) * 8];\n          ",
            "  }\n            \n            double res_5324;\n            double res_5325;\n            \n            if (thread_active_5778) {\n                res_5324 = x_5320 + x_5322;\n                res_5325 = x_5321 + x_5323;\n            }\n            x_5320 = res_5324;\n            x_5321 = res_5325;\n            *(__local double *) &mem_5645[local_tid_5281 * 8] = x_5320;\n            *(__local double *) &mem_5648[local_tid_5281 * 8] = x_5321;\n        }\n        skip_waves_5785 *= 2;\n    }\n    final_result_5316 = x_5320;\n    final_result_5317 = x_5321;\n    if (local_tid_5281 == 0) {\n        *(__global double *) &mem_5651[group_id_5282 * 8] = final_result_5316;\n    }\n    if (local_tid_5281 == 0) {\n        *(__global double *) &mem_5654[group_id_5282 * 8] = final_result_5317;\n    }\n}\n__kernel void chunked_reduce_kernel_5365(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         int32_t sizze_4902,\n                                         int32_t num_threads_5357,\n                                         int32_t per_thread_elements_5360,\n                                         __global\n                                         unsigned char *values_mem_5630,\n                                         __global unsigned char *mem_5636)\n{\n    __local volatile char *restrict mem_5633 = mem_aligned_0;\n    int32_t wave_sizze_5799;\n    int32_t group_sizze_5800;\n    bool thread_active_5801;\n    int32_t global_tid_5365;\n    int32_t local_tid_5366;\n    int32_t group_id_5367;\n    \n    global_tid_5365 = get_global_id(0);\n    local_tid_5366 = get_local_id(0);\n    group_sizze_5800 = get_local_size(0);\n    wave_sizze_5799 = LOCKSTEP_WIDTH;\n    group_id_5367 = get_group_id(0);\n    thread_active_5801 = 1;\n    \n    int32_t chunk_sizze_5372 = smin32(per_thread_elements_5360,\n                                      squot32(sizze_4902 - global_tid_5365 +\n                                              num_threads_5357 - 1,\n                    ",
            "                          num_threads_5357));\n    double res_5375;\n    \n    if (thread_active_5801) {\n        double acc_5378 = 0.0;\n        \n        for (int32_t i_5377 = 0; i_5377 < chunk_sizze_5372; i_5377++) {\n            int32_t j_t_s_5623 = num_threads_5357 * i_5377;\n            int32_t j_p_i_t_s_5624 = global_tid_5365 + j_t_s_5623;\n            double x_5380 = *(__global\n                              double *) &values_mem_5630[j_p_i_t_s_5624 * 8];\n            double res_5383 = acc_5378 + x_5380;\n            double acc_tmp_5802 = res_5383;\n            \n            acc_5378 = acc_tmp_5802;\n        }\n        res_5375 = acc_5378;\n    }\n    \n    double final_result_5386;\n    \n    for (int32_t comb_iter_5803 = 0; comb_iter_5803 < squot32(group_sizze_5349 +\n                                                              group_sizze_5349 -\n                                                              1,\n                                                              group_sizze_5349);\n         comb_iter_5803++) {\n        int32_t combine_id_5370;\n        int32_t flat_comb_id_5804 = comb_iter_5803 * group_sizze_5349 +\n                local_tid_5366;\n        \n        combine_id_5370 = flat_comb_id_5804;\n        if (slt32(combine_id_5370, group_sizze_5349) && 1) {\n            *(__local double *) &mem_5633[combine_id_5370 * 8] = res_5375;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5806;\n    int32_t skip_waves_5805;\n    int32_t my_index_5387;\n    int32_t other_index_5388;\n    double x_5389;\n    double x_5390;\n    \n    my_index_5387 = local_tid_5366;\n    offset_5806 = 0;\n    other_index_5388 = local_tid_5366 + offset_5806;\n    if (slt32(local_tid_5366, group_sizze_5349)) {\n        x_5389 = *(__local double *) &mem_5633[(local_tid_5366 + offset_5806) *\n                                               8];\n    }\n    offset_5806 = 1;\n    other_index_5388 = local_tid_5366 + offset_5806;\n    while (slt32(offset_5806, wave_sizze_5799)) {\n        if (slt32(o",
            "ther_index_5388, group_sizze_5349) && ((local_tid_5366 -\n                                                           squot32(local_tid_5366,\n                                                                   wave_sizze_5799) *\n                                                           wave_sizze_5799) &\n                                                          (2 * offset_5806 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5390 = *(volatile __local\n                           double *) &mem_5633[(local_tid_5366 + offset_5806) *\n                                               8];\n            }\n            \n            double res_5391;\n            \n            if (thread_active_5801) {\n                res_5391 = x_5389 + x_5390;\n            }\n            x_5389 = res_5391;\n            *(volatile __local double *) &mem_5633[local_tid_5366 * 8] = x_5389;\n        }\n        offset_5806 *= 2;\n        other_index_5388 = local_tid_5366 + offset_5806;\n    }\n    skip_waves_5805 = 1;\n    while (slt32(skip_waves_5805, squot32(group_sizze_5349 + wave_sizze_5799 -\n                                          1, wave_sizze_5799))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5806 = skip_waves_5805 * wave_sizze_5799;\n        other_index_5388 = local_tid_5366 + offset_5806;\n        if (slt32(other_index_5388, group_sizze_5349) && ((local_tid_5366 -\n                                                           squot32(local_tid_5366,\n                                                                   wave_sizze_5799) *\n                                                           wave_sizze_5799) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5366,\n                                                                   wave_sizze_5799) &\n                                                           (2 *\n       ",
            "                                                     skip_waves_5805 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5390 = *(__local double *) &mem_5633[(local_tid_5366 +\n                                                        offset_5806) * 8];\n            }\n            \n            double res_5391;\n            \n            if (thread_active_5801) {\n                res_5391 = x_5389 + x_5390;\n            }\n            x_5389 = res_5391;\n            *(__local double *) &mem_5633[local_tid_5366 * 8] = x_5389;\n        }\n        skip_waves_5805 *= 2;\n    }\n    final_result_5386 = x_5389;\n    if (local_tid_5366 == 0) {\n        *(__global double *) &mem_5636[group_id_5367 * 8] = final_result_5386;\n    }\n}\n__kernel void chunked_reduce_kernel_5429(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         __local volatile\n                                         int64_t *mem_aligned_1,\n                                         int32_t sizze_4902, double res_4910,\n                                         int32_t num_threads_5420,\n                                         int32_t per_thread_elements_5423,\n                                         __global\n                                         unsigned char *values_mem_5630,\n                                         __global unsigned char *mem_5651,\n                                         __global unsigned char *mem_5654)\n{\n    __local volatile char *restrict mem_5645 = mem_aligned_0;\n    __local volatile char *restrict mem_5648 = mem_aligned_1;\n    int32_t wave_sizze_5816;\n    int32_t group_sizze_5817;\n    bool thread_active_5818;\n    int32_t global_tid_5429;\n    int32_t local_tid_5430;\n    int32_t group_id_5431;\n    \n    global_tid_5429 = get_global_id(0);\n    local_tid_5430 = get_local_id(0);\n    group_sizze_5817 = get_local_size(0);\n    wave_sizze_5816 = LOCKSTEP_WID",
            "TH;\n    group_id_5431 = get_group_id(0);\n    thread_active_5818 = 1;\n    \n    int32_t chunk_sizze_5440 = smin32(per_thread_elements_5423,\n                                      squot32(sizze_4902 - global_tid_5429 +\n                                              num_threads_5420 - 1,\n                                              num_threads_5420));\n    double res_5444;\n    double res_5445;\n    \n    if (thread_active_5818) {\n        double acc_5448;\n        double acc_5449;\n        \n        acc_5448 = 0.0;\n        acc_5449 = 0.0;\n        for (int32_t i_5447 = 0; i_5447 < chunk_sizze_5440; i_5447++) {\n            int32_t j_t_s_5627 = num_threads_5420 * i_5447;\n            int32_t j_p_i_t_s_5628 = global_tid_5429 + j_t_s_5627;\n            double x_5451 = *(__global\n                              double *) &values_mem_5630[j_p_i_t_s_5628 * 8];\n            double res_5454 = x_5451 - res_4910;\n            double res_5455 = res_5454 * res_5454;\n            double x_5456 = res_5454 * res_5455;\n            double res_5457 = res_5454 * x_5456;\n            double res_5460 = acc_5448 + res_5455;\n            double res_5461 = acc_5449 + res_5457;\n            double acc_tmp_5819 = res_5460;\n            double acc_tmp_5820;\n            \n            acc_tmp_5820 = res_5461;\n            acc_5448 = acc_tmp_5819;\n            acc_5449 = acc_tmp_5820;\n        }\n        res_5444 = acc_5448;\n        res_5445 = acc_5449;\n    }\n    \n    double final_result_5466;\n    double final_result_5467;\n    \n    for (int32_t comb_iter_5821 = 0; comb_iter_5821 < squot32(group_sizze_5412 +\n                                                              group_sizze_5412 -\n                                                              1,\n                                                              group_sizze_5412);\n         comb_iter_5821++) {\n        int32_t combine_id_5436;\n        int32_t flat_comb_id_5822 = comb_iter_5821 * group_sizze_5412 +\n                local_tid_5430;\n        \n        combine_id_5436",
            " = flat_comb_id_5822;\n        if (slt32(combine_id_5436, group_sizze_5412) && 1) {\n            *(__local double *) &mem_5645[combine_id_5436 * 8] = res_5444;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    for (int32_t comb_iter_5823 = 0; comb_iter_5823 < squot32(group_sizze_5412 +\n                                                              group_sizze_5412 -\n                                                              1,\n                                                              group_sizze_5412);\n         comb_iter_5823++) {\n        int32_t combine_id_5437;\n        int32_t flat_comb_id_5824 = comb_iter_5823 * group_sizze_5412 +\n                local_tid_5430;\n        \n        combine_id_5437 = flat_comb_id_5824;\n        if (slt32(combine_id_5437, group_sizze_5412) && 1) {\n            *(__local double *) &mem_5648[combine_id_5437 * 8] = res_5445;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5826;\n    int32_t skip_waves_5825;\n    int32_t my_index_5468;\n    int32_t other_index_5469;\n    double x_5470;\n    double x_5471;\n    double x_5472;\n    double x_5473;\n    \n    my_index_5468 = local_tid_5430;\n    offset_5826 = 0;\n    other_index_5469 = local_tid_5430 + offset_5826;\n    if (slt32(local_tid_5430, group_sizze_5412)) {\n        x_5470 = *(__local double *) &mem_5645[(local_tid_5430 + offset_5826) *\n                                               8];\n        x_5471 = *(__local double *) &mem_5648[(local_tid_5430 + offset_5826) *\n                                               8];\n    }\n    offset_5826 = 1;\n    other_index_5469 = local_tid_5430 + offset_5826;\n    while (slt32(offset_5826, wave_sizze_5816)) {\n        if (slt32(other_index_5469, group_sizze_5412) && ((local_tid_5430 -\n                                                           squot32(local_tid_5430,\n                                                                   wave_sizze_5816) *\n                                                           wave_sizze_5816) &\n     ",
            "                                                     (2 * offset_5826 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5472 = *(volatile __local\n                           double *) &mem_5645[(local_tid_5430 + offset_5826) *\n                                               8];\n                x_5473 = *(volatile __local\n                           double *) &mem_5648[(local_tid_5430 + offset_5826) *\n                                               8];\n            }\n            \n            double res_5474;\n            double res_5475;\n            \n            if (thread_active_5818) {\n                res_5474 = x_5470 + x_5472;\n                res_5475 = x_5471 + x_5473;\n            }\n            x_5470 = res_5474;\n            x_5471 = res_5475;\n            *(volatile __local double *) &mem_5645[local_tid_5430 * 8] = x_5470;\n            *(volatile __local double *) &mem_5648[local_tid_5430 * 8] = x_5471;\n        }\n        offset_5826 *= 2;\n        other_index_5469 = local_tid_5430 + offset_5826;\n    }\n    skip_waves_5825 = 1;\n    while (slt32(skip_waves_5825, squot32(group_sizze_5412 + wave_sizze_5816 -\n                                          1, wave_sizze_5816))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5826 = skip_waves_5825 * wave_sizze_5816;\n        other_index_5469 = local_tid_5430 + offset_5826;\n        if (slt32(other_index_5469, group_sizze_5412) && ((local_tid_5430 -\n                                                           squot32(local_tid_5430,\n                                                                   wave_sizze_5816) *\n                                                           wave_sizze_5816) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5430,\n                                                                   wave_sizze_5816) &\n                    ",
            "                                       (2 *\n                                                            skip_waves_5825 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5472 = *(__local double *) &mem_5645[(local_tid_5430 +\n                                                        offset_5826) * 8];\n                x_5473 = *(__local double *) &mem_5648[(local_tid_5430 +\n                                                        offset_5826) * 8];\n            }\n            \n            double res_5474;\n            double res_5475;\n            \n            if (thread_active_5818) {\n                res_5474 = x_5470 + x_5472;\n                res_5475 = x_5471 + x_5473;\n            }\n            x_5470 = res_5474;\n            x_5471 = res_5475;\n            *(__local double *) &mem_5645[local_tid_5430 * 8] = x_5470;\n            *(__local double *) &mem_5648[local_tid_5430 * 8] = x_5471;\n        }\n        skip_waves_5825 *= 2;\n    }\n    final_result_5466 = x_5470;\n    final_result_5467 = x_5471;\n    if (local_tid_5430 == 0) {\n        *(__global double *) &mem_5651[group_id_5431 * 8] = final_result_5466;\n    }\n    if (local_tid_5430 == 0) {\n        *(__global double *) &mem_5654[group_id_5431 * 8] = final_result_5467;\n    }\n}\n__kernel void chunked_reduce_kernel_5515(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         int32_t sizze_4927,\n                                         int32_t num_threads_5507,\n                                         int32_t per_thread_elements_5510,\n                                         __global\n                                         unsigned char *values_mem_5630,\n                                         __global unsigned char *mem_5636)\n{\n    __local volatile char *restrict mem_5633 = mem_aligned_0;\n    int32_t wave_sizze_5839;\n    int32_t group_sizze_5840;\n    bool thread_active_584",
            "1;\n    int32_t global_tid_5515;\n    int32_t local_tid_5516;\n    int32_t group_id_5517;\n    \n    global_tid_5515 = get_global_id(0);\n    local_tid_5516 = get_local_id(0);\n    group_sizze_5840 = get_local_size(0);\n    wave_sizze_5839 = LOCKSTEP_WIDTH;\n    group_id_5517 = get_group_id(0);\n    thread_active_5841 = 1;\n    \n    int32_t chunk_sizze_5522 = smin32(per_thread_elements_5510,\n                                      squot32(sizze_4927 - global_tid_5515 +\n                                              num_threads_5507 - 1,\n                                              num_threads_5507));\n    double res_5525;\n    \n    if (thread_active_5841) {\n        double acc_5528 = 0.0;\n        \n        for (int32_t i_5527 = 0; i_5527 < chunk_sizze_5522; i_5527++) {\n            int32_t j_t_s_5623 = num_threads_5507 * i_5527;\n            int32_t j_p_i_t_s_5624 = global_tid_5515 + j_t_s_5623;\n            double x_5530 = *(__global\n                              double *) &values_mem_5630[j_p_i_t_s_5624 * 8];\n            double res_5533 = acc_5528 + x_5530;\n            double acc_tmp_5842 = res_5533;\n            \n            acc_5528 = acc_tmp_5842;\n        }\n        res_5525 = acc_5528;\n    }\n    \n    double final_result_5536;\n    \n    for (int32_t comb_iter_5843 = 0; comb_iter_5843 < squot32(group_sizze_5499 +\n                                                              group_sizze_5499 -\n                                                              1,\n                                                              group_sizze_5499);\n         comb_iter_5843++) {\n        int32_t combine_id_5520;\n        int32_t flat_comb_id_5844 = comb_iter_5843 * group_sizze_5499 +\n                local_tid_5516;\n        \n        combine_id_5520 = flat_comb_id_5844;\n        if (slt32(combine_id_5520, group_sizze_5499) && 1) {\n            *(__local double *) &mem_5633[combine_id_5520 * 8] = res_5525;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5846;\n    int32_t skip_wave",
            "s_5845;\n    int32_t my_index_5537;\n    int32_t other_index_5538;\n    double x_5539;\n    double x_5540;\n    \n    my_index_5537 = local_tid_5516;\n    offset_5846 = 0;\n    other_index_5538 = local_tid_5516 + offset_5846;\n    if (slt32(local_tid_5516, group_sizze_5499)) {\n        x_5539 = *(__local double *) &mem_5633[(local_tid_5516 + offset_5846) *\n                                               8];\n    }\n    offset_5846 = 1;\n    other_index_5538 = local_tid_5516 + offset_5846;\n    while (slt32(offset_5846, wave_sizze_5839)) {\n        if (slt32(other_index_5538, group_sizze_5499) && ((local_tid_5516 -\n                                                           squot32(local_tid_5516,\n                                                                   wave_sizze_5839) *\n                                                           wave_sizze_5839) &\n                                                          (2 * offset_5846 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5540 = *(volatile __local\n                           double *) &mem_5633[(local_tid_5516 + offset_5846) *\n                                               8];\n            }\n            \n            double res_5541;\n            \n            if (thread_active_5841) {\n                res_5541 = x_5539 + x_5540;\n            }\n            x_5539 = res_5541;\n            *(volatile __local double *) &mem_5633[local_tid_5516 * 8] = x_5539;\n        }\n        offset_5846 *= 2;\n        other_index_5538 = local_tid_5516 + offset_5846;\n    }\n    skip_waves_5845 = 1;\n    while (slt32(skip_waves_5845, squot32(group_sizze_5499 + wave_sizze_5839 -\n                                          1, wave_sizze_5839))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5846 = skip_waves_5845 * wave_sizze_5839;\n        other_index_5538 = local_tid_5516 + offset_5846;\n        if (slt32(other_index_5538, group_sizze_5499) && ((local_tid_5516 -\n  ",
            "                                                         squot32(local_tid_5516,\n                                                                   wave_sizze_5839) *\n                                                           wave_sizze_5839) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5516,\n                                                                   wave_sizze_5839) &\n                                                           (2 *\n                                                            skip_waves_5845 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5540 = *(__local double *) &mem_5633[(local_tid_5516 +\n                                                        offset_5846) * 8];\n            }\n            \n            double res_5541;\n            \n            if (thread_active_5841) {\n                res_5541 = x_5539 + x_5540;\n            }\n            x_5539 = res_5541;\n            *(__local double *) &mem_5633[local_tid_5516 * 8] = x_5539;\n        }\n        skip_waves_5845 *= 2;\n    }\n    final_result_5536 = x_5539;\n    if (local_tid_5516 == 0) {\n        *(__global double *) &mem_5636[group_id_5517 * 8] = final_result_5536;\n    }\n}\n__kernel void chunked_reduce_kernel_5576(__local volatile\n                                         int64_t *mem_aligned_0,\n                                         int32_t sizze_4927, double res_4935,\n                                         int32_t num_threads_5568,\n                                         int32_t per_thread_elements_5571,\n                                         __global\n                                         unsigned char *values_mem_5630,\n                                         __global unsigned char *mem_5648)\n{\n    __local volatile char *restrict mem_5645 = mem_aligned_0;\n    int32_t wave_sizze_5856;\n    int32_t g",
            "roup_sizze_5857;\n    bool thread_active_5858;\n    int32_t global_tid_5576;\n    int32_t local_tid_5577;\n    int32_t group_id_5578;\n    \n    global_tid_5576 = get_global_id(0);\n    local_tid_5577 = get_local_id(0);\n    group_sizze_5857 = get_local_size(0);\n    wave_sizze_5856 = LOCKSTEP_WIDTH;\n    group_id_5578 = get_group_id(0);\n    thread_active_5858 = 1;\n    \n    int32_t chunk_sizze_5583 = smin32(per_thread_elements_5571,\n                                      squot32(sizze_4927 - global_tid_5576 +\n                                              num_threads_5568 - 1,\n                                              num_threads_5568));\n    double res_5586;\n    \n    if (thread_active_5858) {\n        double acc_5589 = 0.0;\n        \n        for (int32_t i_5588 = 0; i_5588 < chunk_sizze_5583; i_5588++) {\n            int32_t j_t_s_5627 = num_threads_5568 * i_5588;\n            int32_t j_p_i_t_s_5628 = global_tid_5576 + j_t_s_5627;\n            double x_5591 = *(__global\n                              double *) &values_mem_5630[j_p_i_t_s_5628 * 8];\n            double res_5593 = x_5591 - res_4935;\n            double res_5594 = res_5593 * res_5593;\n            double res_5596 = acc_5589 + res_5594;\n            double acc_tmp_5859 = res_5596;\n            \n            acc_5589 = acc_tmp_5859;\n        }\n        res_5586 = acc_5589;\n    }\n    \n    double final_result_5599;\n    \n    for (int32_t comb_iter_5860 = 0; comb_iter_5860 < squot32(group_sizze_5560 +\n                                                              group_sizze_5560 -\n                                                              1,\n                                                              group_sizze_5560);\n         comb_iter_5860++) {\n        int32_t combine_id_5581;\n        int32_t flat_comb_id_5861 = comb_iter_5860 * group_sizze_5560 +\n                local_tid_5577;\n        \n        combine_id_5581 = flat_comb_id_5861;\n        if (slt32(combine_id_5581, group_sizze_5560) && 1) {\n            *(__local double *)",
            " &mem_5645[combine_id_5581 * 8] = res_5586;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5863;\n    int32_t skip_waves_5862;\n    int32_t my_index_5600;\n    int32_t other_index_5601;\n    double x_5602;\n    double x_5603;\n    \n    my_index_5600 = local_tid_5577;\n    offset_5863 = 0;\n    other_index_5601 = local_tid_5577 + offset_5863;\n    if (slt32(local_tid_5577, group_sizze_5560)) {\n        x_5602 = *(__local double *) &mem_5645[(local_tid_5577 + offset_5863) *\n                                               8];\n    }\n    offset_5863 = 1;\n    other_index_5601 = local_tid_5577 + offset_5863;\n    while (slt32(offset_5863, wave_sizze_5856)) {\n        if (slt32(other_index_5601, group_sizze_5560) && ((local_tid_5577 -\n                                                           squot32(local_tid_5577,\n                                                                   wave_sizze_5856) *\n                                                           wave_sizze_5856) &\n                                                          (2 * offset_5863 -\n                                                           1)) == 0) {\n            // read array element\n            {\n                x_5603 = *(volatile __local\n                           double *) &mem_5645[(local_tid_5577 + offset_5863) *\n                                               8];\n            }\n            \n            double res_5604;\n            \n            if (thread_active_5858) {\n                res_5604 = x_5602 + x_5603;\n            }\n            x_5602 = res_5604;\n            *(volatile __local double *) &mem_5645[local_tid_5577 * 8] = x_5602;\n        }\n        offset_5863 *= 2;\n        other_index_5601 = local_tid_5577 + offset_5863;\n    }\n    skip_waves_5862 = 1;\n    while (slt32(skip_waves_5862, squot32(group_sizze_5560 + wave_sizze_5856 -\n                                          1, wave_sizze_5856))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5863 = skip_waves_5862 * wave_si",
            "zze_5856;\n        other_index_5601 = local_tid_5577 + offset_5863;\n        if (slt32(other_index_5601, group_sizze_5560) && ((local_tid_5577 -\n                                                           squot32(local_tid_5577,\n                                                                   wave_sizze_5856) *\n                                                           wave_sizze_5856) ==\n                                                          0 &&\n                                                          (squot32(local_tid_5577,\n                                                                   wave_sizze_5856) &\n                                                           (2 *\n                                                            skip_waves_5862 -\n                                                            1)) == 0)) {\n            // read array element\n            {\n                x_5603 = *(__local double *) &mem_5645[(local_tid_5577 +\n                                                        offset_5863) * 8];\n            }\n            \n            double res_5604;\n            \n            if (thread_active_5858) {\n                res_5604 = x_5602 + x_5603;\n            }\n            x_5602 = res_5604;\n            *(__local double *) &mem_5645[local_tid_5577 * 8] = x_5602;\n        }\n        skip_waves_5862 *= 2;\n    }\n    final_result_5599 = x_5602;\n    if (local_tid_5577 == 0) {\n        *(__global double *) &mem_5648[group_id_5578 * 8] = final_result_5599;\n    }\n}\n__kernel void reduce_kernel_4998(__local volatile int64_t *mem_aligned_0,\n                                 int32_t num_groups_4961, __global\n                                 unsigned char *mem_5636, __global\n                                 unsigned char *mem_5642)\n{\n    __local volatile char *restrict mem_5639 = mem_aligned_0;\n    int32_t wave_sizze_5697;\n    int32_t group_sizze_5698;\n    bool thread_active_5699;\n    int32_t global_tid_4998;\n    int32_t local_tid_4999;\n    int32_t group_id_5000;\n",
            "    \n    global_tid_4998 = get_global_id(0);\n    local_tid_4999 = get_local_id(0);\n    group_sizze_5698 = get_local_size(0);\n    wave_sizze_5697 = LOCKSTEP_WIDTH;\n    group_id_5000 = get_group_id(0);\n    thread_active_5699 = 1;\n    \n    bool in_bounds_5001;\n    double x_5615;\n    \n    if (thread_active_5699) {\n        in_bounds_5001 = slt32(local_tid_4999, num_groups_4961);\n        if (in_bounds_5001) {\n            double x_5002 = *(__global double *) &mem_5636[global_tid_4998 * 8];\n            \n            x_5615 = x_5002;\n        } else {\n            x_5615 = 0.0;\n        }\n    }\n    \n    double final_result_5006;\n    \n    for (int32_t comb_iter_5700 = 0; comb_iter_5700 <\n         squot32(max_num_groups_4956 + max_num_groups_4956 - 1,\n                 max_num_groups_4956); comb_iter_5700++) {\n        int32_t combine_id_5005;\n        int32_t flat_comb_id_5701 = comb_iter_5700 * max_num_groups_4956 +\n                local_tid_4999;\n        \n        combine_id_5005 = flat_comb_id_5701;\n        if (slt32(combine_id_5005, max_num_groups_4956) && 1) {\n            *(__local double *) &mem_5639[combine_id_5005 * 8] = x_5615;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5703;\n    int32_t skip_waves_5702;\n    double x_4844;\n    double x_4845;\n    int32_t my_index_4968;\n    int32_t other_index_4969;\n    \n    my_index_4968 = local_tid_4999;\n    offset_5703 = 0;\n    other_index_4969 = local_tid_4999 + offset_5703;\n    if (slt32(local_tid_4999, max_num_groups_4956)) {\n        x_4844 = *(__local double *) &mem_5639[(local_tid_4999 + offset_5703) *\n                                               8];\n    }\n    offset_5703 = 1;\n    other_index_4969 = local_tid_4999 + offset_5703;\n    while (slt32(offset_5703, wave_sizze_5697)) {\n        if (slt32(other_index_4969, max_num_groups_4956) && ((local_tid_4999 -\n                                                              squot32(local_tid_4999,\n                                                                ",
            "      wave_sizze_5697) *\n                                                              wave_sizze_5697) &\n                                                             (2 * offset_5703 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4845 = *(volatile __local\n                           double *) &mem_5639[(local_tid_4999 + offset_5703) *\n                                               8];\n            }\n            \n            double res_4846;\n            \n            if (thread_active_5699) {\n                res_4846 = x_4844 + x_4845;\n            }\n            x_4844 = res_4846;\n            *(volatile __local double *) &mem_5639[local_tid_4999 * 8] = x_4844;\n        }\n        offset_5703 *= 2;\n        other_index_4969 = local_tid_4999 + offset_5703;\n    }\n    skip_waves_5702 = 1;\n    while (slt32(skip_waves_5702, squot32(max_num_groups_4956 +\n                                          wave_sizze_5697 - 1,\n                                          wave_sizze_5697))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5703 = skip_waves_5702 * wave_sizze_5697;\n        other_index_4969 = local_tid_4999 + offset_5703;\n        if (slt32(other_index_4969, max_num_groups_4956) && ((local_tid_4999 -\n                                                              squot32(local_tid_4999,\n                                                                      wave_sizze_5697) *\n                                                              wave_sizze_5697) ==\n                                                             0 &&\n                                                             (squot32(local_tid_4999,\n                                                                      wave_sizze_5697) &\n                                                              (2 *\n                                                               skip_waves_5702 -\n                                                 ",
            "              1)) == 0)) {\n            // read array element\n            {\n                x_4845 = *(__local double *) &mem_5639[(local_tid_4999 +\n                                                        offset_5703) * 8];\n            }\n            \n            double res_4846;\n            \n            if (thread_active_5699) {\n                res_4846 = x_4844 + x_4845;\n            }\n            x_4844 = res_4846;\n            *(__local double *) &mem_5639[local_tid_4999 * 8] = x_4844;\n        }\n        skip_waves_5702 *= 2;\n    }\n    final_result_5006 = x_4844;\n    if (local_tid_4999 == 0) {\n        *(__global double *) &mem_5642[group_id_5000 * 8] = final_result_5006;\n    }\n}\n__kernel void reduce_kernel_5059(__local volatile int64_t *mem_aligned_0,\n                                 int32_t num_groups_5022, __global\n                                 unsigned char *mem_5636, __global\n                                 unsigned char *mem_5642)\n{\n    __local volatile char *restrict mem_5639 = mem_aligned_0;\n    int32_t wave_sizze_5715;\n    int32_t group_sizze_5716;\n    bool thread_active_5717;\n    int32_t global_tid_5059;\n    int32_t local_tid_5060;\n    int32_t group_id_5061;\n    \n    global_tid_5059 = get_global_id(0);\n    local_tid_5060 = get_local_id(0);\n    group_sizze_5716 = get_local_size(0);\n    wave_sizze_5715 = LOCKSTEP_WIDTH;\n    group_id_5061 = get_group_id(0);\n    thread_active_5717 = 1;\n    \n    bool in_bounds_5062;\n    double x_5615;\n    \n    if (thread_active_5717) {\n        in_bounds_5062 = slt32(local_tid_5060, num_groups_5022);\n        if (in_bounds_5062) {\n            double x_5063 = *(__global double *) &mem_5636[global_tid_5059 * 8];\n            \n            x_5615 = x_5063;\n        } else {\n            x_5615 = 0.0;\n        }\n    }\n    \n    double final_result_5067;\n    \n    for (int32_t comb_iter_5718 = 0; comb_iter_5718 <\n         squot32(max_num_groups_5017 + max_num_groups_5017 - 1,\n                 max_num_groups_5017); comb_iter_5718++) {\n     ",
            "   int32_t combine_id_5066;\n        int32_t flat_comb_id_5719 = comb_iter_5718 * max_num_groups_5017 +\n                local_tid_5060;\n        \n        combine_id_5066 = flat_comb_id_5719;\n        if (slt32(combine_id_5066, max_num_groups_5017) && 1) {\n            *(__local double *) &mem_5639[combine_id_5066 * 8] = x_5615;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5721;\n    int32_t skip_waves_5720;\n    double x_4851;\n    double x_4852;\n    int32_t my_index_5029;\n    int32_t other_index_5030;\n    \n    my_index_5029 = local_tid_5060;\n    offset_5721 = 0;\n    other_index_5030 = local_tid_5060 + offset_5721;\n    if (slt32(local_tid_5060, max_num_groups_5017)) {\n        x_4851 = *(__local double *) &mem_5639[(local_tid_5060 + offset_5721) *\n                                               8];\n    }\n    offset_5721 = 1;\n    other_index_5030 = local_tid_5060 + offset_5721;\n    while (slt32(offset_5721, wave_sizze_5715)) {\n        if (slt32(other_index_5030, max_num_groups_5017) && ((local_tid_5060 -\n                                                              squot32(local_tid_5060,\n                                                                      wave_sizze_5715) *\n                                                              wave_sizze_5715) &\n                                                             (2 * offset_5721 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4852 = *(volatile __local\n                           double *) &mem_5639[(local_tid_5060 + offset_5721) *\n                                               8];\n            }\n            \n            double res_4853;\n            \n            if (thread_active_5717) {\n                res_4853 = x_4851 + x_4852;\n            }\n            x_4851 = res_4853;\n            *(volatile __local double *) &mem_5639[local_tid_5060 * 8] = x_4851;\n        }\n        offset_5721 *= 2;\n        other_i",
            "ndex_5030 = local_tid_5060 + offset_5721;\n    }\n    skip_waves_5720 = 1;\n    while (slt32(skip_waves_5720, squot32(max_num_groups_5017 +\n                                          wave_sizze_5715 - 1,\n                                          wave_sizze_5715))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5721 = skip_waves_5720 * wave_sizze_5715;\n        other_index_5030 = local_tid_5060 + offset_5721;\n        if (slt32(other_index_5030, max_num_groups_5017) && ((local_tid_5060 -\n                                                              squot32(local_tid_5060,\n                                                                      wave_sizze_5715) *\n                                                              wave_sizze_5715) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5060,\n                                                                      wave_sizze_5715) &\n                                                              (2 *\n                                                               skip_waves_5720 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4852 = *(__local double *) &mem_5639[(local_tid_5060 +\n                                                        offset_5721) * 8];\n            }\n            \n            double res_4853;\n            \n            if (thread_active_5717) {\n                res_4853 = x_4851 + x_4852;\n            }\n            x_4851 = res_4853;\n            *(__local double *) &mem_5639[local_tid_5060 * 8] = x_4851;\n        }\n        skip_waves_5720 *= 2;\n    }\n    final_result_5067 = x_4851;\n    if (local_tid_5060 == 0) {\n        *(__global double *) &mem_5642[group_id_5061 * 8] = final_result_5067;\n    }\n}\n__kernel void reduce_kernel_5120(__local volatile int64_t *mem_aligned_0,\n                                 int32_t num_group",
            "s_5083, __global\n                                 unsigned char *mem_5636, __global\n                                 unsigned char *mem_5642)\n{\n    __local volatile char *restrict mem_5639 = mem_aligned_0;\n    int32_t wave_sizze_5733;\n    int32_t group_sizze_5734;\n    bool thread_active_5735;\n    int32_t global_tid_5120;\n    int32_t local_tid_5121;\n    int32_t group_id_5122;\n    \n    global_tid_5120 = get_global_id(0);\n    local_tid_5121 = get_local_id(0);\n    group_sizze_5734 = get_local_size(0);\n    wave_sizze_5733 = LOCKSTEP_WIDTH;\n    group_id_5122 = get_group_id(0);\n    thread_active_5735 = 1;\n    \n    bool in_bounds_5123;\n    double x_5615;\n    \n    if (thread_active_5735) {\n        in_bounds_5123 = slt32(local_tid_5121, num_groups_5083);\n        if (in_bounds_5123) {\n            double x_5124 = *(__global double *) &mem_5636[global_tid_5120 * 8];\n            \n            x_5615 = x_5124;\n        } else {\n            x_5615 = 0.0;\n        }\n    }\n    \n    double final_result_5128;\n    \n    for (int32_t comb_iter_5736 = 0; comb_iter_5736 <\n         squot32(max_num_groups_5078 + max_num_groups_5078 - 1,\n                 max_num_groups_5078); comb_iter_5736++) {\n        int32_t combine_id_5127;\n        int32_t flat_comb_id_5737 = comb_iter_5736 * max_num_groups_5078 +\n                local_tid_5121;\n        \n        combine_id_5127 = flat_comb_id_5737;\n        if (slt32(combine_id_5127, max_num_groups_5078) && 1) {\n            *(__local double *) &mem_5639[combine_id_5127 * 8] = x_5615;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5739;\n    int32_t skip_waves_5738;\n    double x_4861;\n    double x_4862;\n    int32_t my_index_5090;\n    int32_t other_index_5091;\n    \n    my_index_5090 = local_tid_5121;\n    offset_5739 = 0;\n    other_index_5091 = local_tid_5121 + offset_5739;\n    if (slt32(local_tid_5121, max_num_groups_5078)) {\n        x_4861 = *(__local double *) &mem_5639[(local_tid_5121 + offset_5739) *\n                                ",
            "               8];\n    }\n    offset_5739 = 1;\n    other_index_5091 = local_tid_5121 + offset_5739;\n    while (slt32(offset_5739, wave_sizze_5733)) {\n        if (slt32(other_index_5091, max_num_groups_5078) && ((local_tid_5121 -\n                                                              squot32(local_tid_5121,\n                                                                      wave_sizze_5733) *\n                                                              wave_sizze_5733) &\n                                                             (2 * offset_5739 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4862 = *(volatile __local\n                           double *) &mem_5639[(local_tid_5121 + offset_5739) *\n                                               8];\n            }\n            \n            double res_4863;\n            \n            if (thread_active_5735) {\n                res_4863 = x_4861 + x_4862;\n            }\n            x_4861 = res_4863;\n            *(volatile __local double *) &mem_5639[local_tid_5121 * 8] = x_4861;\n        }\n        offset_5739 *= 2;\n        other_index_5091 = local_tid_5121 + offset_5739;\n    }\n    skip_waves_5738 = 1;\n    while (slt32(skip_waves_5738, squot32(max_num_groups_5078 +\n                                          wave_sizze_5733 - 1,\n                                          wave_sizze_5733))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5739 = skip_waves_5738 * wave_sizze_5733;\n        other_index_5091 = local_tid_5121 + offset_5739;\n        if (slt32(other_index_5091, max_num_groups_5078) && ((local_tid_5121 -\n                                                              squot32(local_tid_5121,\n                                                                      wave_sizze_5733) *\n                                                              wave_sizze_5733) ==\n                                                            ",
            " 0 &&\n                                                             (squot32(local_tid_5121,\n                                                                      wave_sizze_5733) &\n                                                              (2 *\n                                                               skip_waves_5738 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4862 = *(__local double *) &mem_5639[(local_tid_5121 +\n                                                        offset_5739) * 8];\n            }\n            \n            double res_4863;\n            \n            if (thread_active_5735) {\n                res_4863 = x_4861 + x_4862;\n            }\n            x_4861 = res_4863;\n            *(__local double *) &mem_5639[local_tid_5121 * 8] = x_4861;\n        }\n        skip_waves_5738 *= 2;\n    }\n    final_result_5128 = x_4861;\n    if (local_tid_5121 == 0) {\n        *(__global double *) &mem_5642[group_id_5122 * 8] = final_result_5128;\n    }\n}\n__kernel void reduce_kernel_5183(__local volatile int64_t *mem_aligned_0,\n                                 int32_t num_groups_5144, __global\n                                 unsigned char *mem_5648, __global\n                                 unsigned char *mem_5654)\n{\n    __local volatile char *restrict mem_5651 = mem_aligned_0;\n    int32_t wave_sizze_5750;\n    int32_t group_sizze_5751;\n    bool thread_active_5752;\n    int32_t global_tid_5183;\n    int32_t local_tid_5184;\n    int32_t group_id_5185;\n    \n    global_tid_5183 = get_global_id(0);\n    local_tid_5184 = get_local_id(0);\n    group_sizze_5751 = get_local_size(0);\n    wave_sizze_5750 = LOCKSTEP_WIDTH;\n    group_id_5185 = get_group_id(0);\n    thread_active_5752 = 1;\n    \n    bool in_bounds_5186;\n    double x_5617;\n    \n    if (thread_active_5752) {\n        in_bounds_5186 = slt32(local_tid_5184, num_groups_5144);\n        if (in_bounds_5186) {\n            double x_5187 ",
            "= *(__global double *) &mem_5648[global_tid_5183 * 8];\n            \n            x_5617 = x_5187;\n        } else {\n            x_5617 = 0.0;\n        }\n    }\n    \n    double final_result_5191;\n    \n    for (int32_t comb_iter_5753 = 0; comb_iter_5753 <\n         squot32(max_num_groups_5139 + max_num_groups_5139 - 1,\n                 max_num_groups_5139); comb_iter_5753++) {\n        int32_t combine_id_5190;\n        int32_t flat_comb_id_5754 = comb_iter_5753 * max_num_groups_5139 +\n                local_tid_5184;\n        \n        combine_id_5190 = flat_comb_id_5754;\n        if (slt32(combine_id_5190, max_num_groups_5139) && 1) {\n            *(__local double *) &mem_5651[combine_id_5190 * 8] = x_5617;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5756;\n    int32_t skip_waves_5755;\n    double x_4867;\n    double x_4868;\n    int32_t my_index_5151;\n    int32_t other_index_5152;\n    \n    my_index_5151 = local_tid_5184;\n    offset_5756 = 0;\n    other_index_5152 = local_tid_5184 + offset_5756;\n    if (slt32(local_tid_5184, max_num_groups_5139)) {\n        x_4867 = *(__local double *) &mem_5651[(local_tid_5184 + offset_5756) *\n                                               8];\n    }\n    offset_5756 = 1;\n    other_index_5152 = local_tid_5184 + offset_5756;\n    while (slt32(offset_5756, wave_sizze_5750)) {\n        if (slt32(other_index_5152, max_num_groups_5139) && ((local_tid_5184 -\n                                                              squot32(local_tid_5184,\n                                                                      wave_sizze_5750) *\n                                                              wave_sizze_5750) &\n                                                             (2 * offset_5756 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4868 = *(volatile __local\n                           double *) &mem_5651[(local_tid_5184 + offset_5756) *\n ",
            "                                              8];\n            }\n            \n            double res_4869;\n            \n            if (thread_active_5752) {\n                res_4869 = x_4867 + x_4868;\n            }\n            x_4867 = res_4869;\n            *(volatile __local double *) &mem_5651[local_tid_5184 * 8] = x_4867;\n        }\n        offset_5756 *= 2;\n        other_index_5152 = local_tid_5184 + offset_5756;\n    }\n    skip_waves_5755 = 1;\n    while (slt32(skip_waves_5755, squot32(max_num_groups_5139 +\n                                          wave_sizze_5750 - 1,\n                                          wave_sizze_5750))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5756 = skip_waves_5755 * wave_sizze_5750;\n        other_index_5152 = local_tid_5184 + offset_5756;\n        if (slt32(other_index_5152, max_num_groups_5139) && ((local_tid_5184 -\n                                                              squot32(local_tid_5184,\n                                                                      wave_sizze_5750) *\n                                                              wave_sizze_5750) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5184,\n                                                                      wave_sizze_5750) &\n                                                              (2 *\n                                                               skip_waves_5755 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4868 = *(__local double *) &mem_5651[(local_tid_5184 +\n                                                        offset_5756) * 8];\n            }\n            \n            double res_4869;\n            \n            if (thread_active_5752) {\n                res_4869 = x_4867 + x_4868;\n            }\n            x_4867 = res_4869;\n            *(",
            "__local double *) &mem_5651[local_tid_5184 * 8] = x_4867;\n        }\n        skip_waves_5755 *= 2;\n    }\n    final_result_5191 = x_4867;\n    if (local_tid_5184 == 0) {\n        *(__global double *) &mem_5654[group_id_5185 * 8] = final_result_5191;\n    }\n}\n__kernel void reduce_kernel_5244(__local volatile int64_t *mem_aligned_0,\n                                 int32_t num_groups_5207, __global\n                                 unsigned char *mem_5636, __global\n                                 unsigned char *mem_5642)\n{\n    __local volatile char *restrict mem_5639 = mem_aligned_0;\n    int32_t wave_sizze_5768;\n    int32_t group_sizze_5769;\n    bool thread_active_5770;\n    int32_t global_tid_5244;\n    int32_t local_tid_5245;\n    int32_t group_id_5246;\n    \n    global_tid_5244 = get_global_id(0);\n    local_tid_5245 = get_local_id(0);\n    group_sizze_5769 = get_local_size(0);\n    wave_sizze_5768 = LOCKSTEP_WIDTH;\n    group_id_5246 = get_group_id(0);\n    thread_active_5770 = 1;\n    \n    bool in_bounds_5247;\n    double x_5615;\n    \n    if (thread_active_5770) {\n        in_bounds_5247 = slt32(local_tid_5245, num_groups_5207);\n        if (in_bounds_5247) {\n            double x_5248 = *(__global double *) &mem_5636[global_tid_5244 * 8];\n            \n            x_5615 = x_5248;\n        } else {\n            x_5615 = 0.0;\n        }\n    }\n    \n    double final_result_5252;\n    \n    for (int32_t comb_iter_5771 = 0; comb_iter_5771 <\n         squot32(max_num_groups_5202 + max_num_groups_5202 - 1,\n                 max_num_groups_5202); comb_iter_5771++) {\n        int32_t combine_id_5251;\n        int32_t flat_comb_id_5772 = comb_iter_5771 * max_num_groups_5202 +\n                local_tid_5245;\n        \n        combine_id_5251 = flat_comb_id_5772;\n        if (slt32(combine_id_5251, max_num_groups_5202) && 1) {\n            *(__local double *) &mem_5639[combine_id_5251 * 8] = x_5615;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5774;\n    int32_t skip_waves_5773",
            ";\n    double x_4879;\n    double x_4880;\n    int32_t my_index_5214;\n    int32_t other_index_5215;\n    \n    my_index_5214 = local_tid_5245;\n    offset_5774 = 0;\n    other_index_5215 = local_tid_5245 + offset_5774;\n    if (slt32(local_tid_5245, max_num_groups_5202)) {\n        x_4879 = *(__local double *) &mem_5639[(local_tid_5245 + offset_5774) *\n                                               8];\n    }\n    offset_5774 = 1;\n    other_index_5215 = local_tid_5245 + offset_5774;\n    while (slt32(offset_5774, wave_sizze_5768)) {\n        if (slt32(other_index_5215, max_num_groups_5202) && ((local_tid_5245 -\n                                                              squot32(local_tid_5245,\n                                                                      wave_sizze_5768) *\n                                                              wave_sizze_5768) &\n                                                             (2 * offset_5774 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4880 = *(volatile __local\n                           double *) &mem_5639[(local_tid_5245 + offset_5774) *\n                                               8];\n            }\n            \n            double res_4881;\n            \n            if (thread_active_5770) {\n                res_4881 = x_4879 + x_4880;\n            }\n            x_4879 = res_4881;\n            *(volatile __local double *) &mem_5639[local_tid_5245 * 8] = x_4879;\n        }\n        offset_5774 *= 2;\n        other_index_5215 = local_tid_5245 + offset_5774;\n    }\n    skip_waves_5773 = 1;\n    while (slt32(skip_waves_5773, squot32(max_num_groups_5202 +\n                                          wave_sizze_5768 - 1,\n                                          wave_sizze_5768))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5774 = skip_waves_5773 * wave_sizze_5768;\n        other_index_5215 = local_tid_5245 + offset_5774;\n        if (slt32(",
            "other_index_5215, max_num_groups_5202) && ((local_tid_5245 -\n                                                              squot32(local_tid_5245,\n                                                                      wave_sizze_5768) *\n                                                              wave_sizze_5768) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5245,\n                                                                      wave_sizze_5768) &\n                                                              (2 *\n                                                               skip_waves_5773 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4880 = *(__local double *) &mem_5639[(local_tid_5245 +\n                                                        offset_5774) * 8];\n            }\n            \n            double res_4881;\n            \n            if (thread_active_5770) {\n                res_4881 = x_4879 + x_4880;\n            }\n            x_4879 = res_4881;\n            *(__local double *) &mem_5639[local_tid_5245 * 8] = x_4879;\n        }\n        skip_waves_5773 *= 2;\n    }\n    final_result_5252 = x_4879;\n    if (local_tid_5245 == 0) {\n        *(__global double *) &mem_5642[group_id_5246 * 8] = final_result_5252;\n    }\n}\n__kernel void reduce_kernel_5328(__local volatile int64_t *mem_aligned_0,\n                                 __local volatile int64_t *mem_aligned_1,\n                                 int32_t num_groups_5270, __global\n                                 unsigned char *mem_5651, __global\n                                 unsigned char *mem_5654, __global\n                                 unsigned char *mem_5663, __global\n                                 unsigned char *mem_5666)\n{\n    __local volatile char *restrict mem_5657 = mem_aligned_0;\n    __local volatil",
            "e char *restrict mem_5660 = mem_aligned_1;\n    int32_t wave_sizze_5789;\n    int32_t group_sizze_5790;\n    bool thread_active_5791;\n    int32_t global_tid_5328;\n    int32_t local_tid_5329;\n    int32_t group_id_5330;\n    \n    global_tid_5328 = get_global_id(0);\n    local_tid_5329 = get_local_id(0);\n    group_sizze_5790 = get_local_size(0);\n    wave_sizze_5789 = LOCKSTEP_WIDTH;\n    group_id_5330 = get_group_id(0);\n    thread_active_5791 = 1;\n    \n    bool in_bounds_5331;\n    double x_5617;\n    double x_5619;\n    \n    if (thread_active_5791) {\n        in_bounds_5331 = slt32(local_tid_5329, num_groups_5270);\n        if (in_bounds_5331) {\n            double x_5332 = *(__global double *) &mem_5651[global_tid_5328 * 8];\n            \n            x_5617 = x_5332;\n        } else {\n            x_5617 = 0.0;\n        }\n        if (in_bounds_5331) {\n            double x_5334 = *(__global double *) &mem_5654[global_tid_5328 * 8];\n            \n            x_5619 = x_5334;\n        } else {\n            x_5619 = 0.0;\n        }\n    }\n    \n    double final_result_5339;\n    double final_result_5340;\n    \n    for (int32_t comb_iter_5792 = 0; comb_iter_5792 <\n         squot32(max_num_groups_5265 + max_num_groups_5265 - 1,\n                 max_num_groups_5265); comb_iter_5792++) {\n        int32_t combine_id_5338;\n        int32_t flat_comb_id_5793 = comb_iter_5792 * max_num_groups_5265 +\n                local_tid_5329;\n        \n        combine_id_5338 = flat_comb_id_5793;\n        if (slt32(combine_id_5338, max_num_groups_5265) && 1) {\n            *(__local double *) &mem_5657[combine_id_5338 * 8] = x_5617;\n            *(__local double *) &mem_5660[combine_id_5338 * 8] = x_5619;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5795;\n    int32_t skip_waves_5794;\n    double x_4886;\n    double x_4887;\n    double x_4888;\n    double x_4889;\n    int32_t my_index_5278;\n    int32_t other_index_5279;\n    \n    my_index_5278 = local_tid_5329;\n    offset_5795 = 0;\n    other_index_",
            "5279 = local_tid_5329 + offset_5795;\n    if (slt32(local_tid_5329, max_num_groups_5265)) {\n        x_4886 = *(__local double *) &mem_5657[(local_tid_5329 + offset_5795) *\n                                               8];\n        x_4887 = *(__local double *) &mem_5660[(local_tid_5329 + offset_5795) *\n                                               8];\n    }\n    offset_5795 = 1;\n    other_index_5279 = local_tid_5329 + offset_5795;\n    while (slt32(offset_5795, wave_sizze_5789)) {\n        if (slt32(other_index_5279, max_num_groups_5265) && ((local_tid_5329 -\n                                                              squot32(local_tid_5329,\n                                                                      wave_sizze_5789) *\n                                                              wave_sizze_5789) &\n                                                             (2 * offset_5795 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4888 = *(volatile __local\n                           double *) &mem_5657[(local_tid_5329 + offset_5795) *\n                                               8];\n                x_4889 = *(volatile __local\n                           double *) &mem_5660[(local_tid_5329 + offset_5795) *\n                                               8];\n            }\n            \n            double res_4890;\n            double res_4891;\n            \n            if (thread_active_5791) {\n                res_4890 = x_4886 + x_4888;\n                res_4891 = x_4887 + x_4889;\n            }\n            x_4886 = res_4890;\n            x_4887 = res_4891;\n            *(volatile __local double *) &mem_5657[local_tid_5329 * 8] = x_4886;\n            *(volatile __local double *) &mem_5660[local_tid_5329 * 8] = x_4887;\n        }\n        offset_5795 *= 2;\n        other_index_5279 = local_tid_5329 + offset_5795;\n    }\n    skip_waves_5794 = 1;\n    while (slt32(skip_waves_5794, squot32(ma",
            "x_num_groups_5265 +\n                                          wave_sizze_5789 - 1,\n                                          wave_sizze_5789))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5795 = skip_waves_5794 * wave_sizze_5789;\n        other_index_5279 = local_tid_5329 + offset_5795;\n        if (slt32(other_index_5279, max_num_groups_5265) && ((local_tid_5329 -\n                                                              squot32(local_tid_5329,\n                                                                      wave_sizze_5789) *\n                                                              wave_sizze_5789) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5329,\n                                                                      wave_sizze_5789) &\n                                                              (2 *\n                                                               skip_waves_5794 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4888 = *(__local double *) &mem_5657[(local_tid_5329 +\n                                                        offset_5795) * 8];\n                x_4889 = *(__local double *) &mem_5660[(local_tid_5329 +\n                                                        offset_5795) * 8];\n            }\n            \n            double res_4890;\n            double res_4891;\n            \n            if (thread_active_5791) {\n                res_4890 = x_4886 + x_4888;\n                res_4891 = x_4887 + x_4889;\n            }\n            x_4886 = res_4890;\n            x_4887 = res_4891;\n            *(__local double *) &mem_5657[local_tid_5329 * 8] = x_4886;\n            *(__local double *) &mem_5660[local_tid_5329 * 8] = x_4887;\n        }\n        skip_waves_5794 *= 2;\n    }\n    final_result_5339 = x_4886;\n    final_result_5340 = x_4887;\n   ",
            " if (local_tid_5329 == 0) {\n        *(__global double *) &mem_5663[group_id_5330 * 8] = final_result_5339;\n    }\n    if (local_tid_5329 == 0) {\n        *(__global double *) &mem_5666[group_id_5330 * 8] = final_result_5340;\n    }\n}\n__kernel void reduce_kernel_5393(__local volatile int64_t *mem_aligned_0,\n                                 int32_t num_groups_5356, __global\n                                 unsigned char *mem_5636, __global\n                                 unsigned char *mem_5642)\n{\n    __local volatile char *restrict mem_5639 = mem_aligned_0;\n    int32_t wave_sizze_5808;\n    int32_t group_sizze_5809;\n    bool thread_active_5810;\n    int32_t global_tid_5393;\n    int32_t local_tid_5394;\n    int32_t group_id_5395;\n    \n    global_tid_5393 = get_global_id(0);\n    local_tid_5394 = get_local_id(0);\n    group_sizze_5809 = get_local_size(0);\n    wave_sizze_5808 = LOCKSTEP_WIDTH;\n    group_id_5395 = get_group_id(0);\n    thread_active_5810 = 1;\n    \n    bool in_bounds_5396;\n    double x_5615;\n    \n    if (thread_active_5810) {\n        in_bounds_5396 = slt32(local_tid_5394, num_groups_5356);\n        if (in_bounds_5396) {\n            double x_5397 = *(__global double *) &mem_5636[global_tid_5393 * 8];\n            \n            x_5615 = x_5397;\n        } else {\n            x_5615 = 0.0;\n        }\n    }\n    \n    double final_result_5401;\n    \n    for (int32_t comb_iter_5811 = 0; comb_iter_5811 <\n         squot32(max_num_groups_5351 + max_num_groups_5351 - 1,\n                 max_num_groups_5351); comb_iter_5811++) {\n        int32_t combine_id_5400;\n        int32_t flat_comb_id_5812 = comb_iter_5811 * max_num_groups_5351 +\n                local_tid_5394;\n        \n        combine_id_5400 = flat_comb_id_5812;\n        if (slt32(combine_id_5400, max_num_groups_5351) && 1) {\n            *(__local double *) &mem_5639[combine_id_5400 * 8] = x_5615;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5814;\n    int32_t skip_waves_5813;\n    double x_4906;\n  ",
            "  double x_4907;\n    int32_t my_index_5363;\n    int32_t other_index_5364;\n    \n    my_index_5363 = local_tid_5394;\n    offset_5814 = 0;\n    other_index_5364 = local_tid_5394 + offset_5814;\n    if (slt32(local_tid_5394, max_num_groups_5351)) {\n        x_4906 = *(__local double *) &mem_5639[(local_tid_5394 + offset_5814) *\n                                               8];\n    }\n    offset_5814 = 1;\n    other_index_5364 = local_tid_5394 + offset_5814;\n    while (slt32(offset_5814, wave_sizze_5808)) {\n        if (slt32(other_index_5364, max_num_groups_5351) && ((local_tid_5394 -\n                                                              squot32(local_tid_5394,\n                                                                      wave_sizze_5808) *\n                                                              wave_sizze_5808) &\n                                                             (2 * offset_5814 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4907 = *(volatile __local\n                           double *) &mem_5639[(local_tid_5394 + offset_5814) *\n                                               8];\n            }\n            \n            double res_4908;\n            \n            if (thread_active_5810) {\n                res_4908 = x_4906 + x_4907;\n            }\n            x_4906 = res_4908;\n            *(volatile __local double *) &mem_5639[local_tid_5394 * 8] = x_4906;\n        }\n        offset_5814 *= 2;\n        other_index_5364 = local_tid_5394 + offset_5814;\n    }\n    skip_waves_5813 = 1;\n    while (slt32(skip_waves_5813, squot32(max_num_groups_5351 +\n                                          wave_sizze_5808 - 1,\n                                          wave_sizze_5808))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5814 = skip_waves_5813 * wave_sizze_5808;\n        other_index_5364 = local_tid_5394 + offset_5814;\n        if (slt32(other_index_5364, max_n",
            "um_groups_5351) && ((local_tid_5394 -\n                                                              squot32(local_tid_5394,\n                                                                      wave_sizze_5808) *\n                                                              wave_sizze_5808) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5394,\n                                                                      wave_sizze_5808) &\n                                                              (2 *\n                                                               skip_waves_5813 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4907 = *(__local double *) &mem_5639[(local_tid_5394 +\n                                                        offset_5814) * 8];\n            }\n            \n            double res_4908;\n            \n            if (thread_active_5810) {\n                res_4908 = x_4906 + x_4907;\n            }\n            x_4906 = res_4908;\n            *(__local double *) &mem_5639[local_tid_5394 * 8] = x_4906;\n        }\n        skip_waves_5813 *= 2;\n    }\n    final_result_5401 = x_4906;\n    if (local_tid_5394 == 0) {\n        *(__global double *) &mem_5642[group_id_5395 * 8] = final_result_5401;\n    }\n}\n__kernel void reduce_kernel_5478(__local volatile int64_t *mem_aligned_0,\n                                 __local volatile int64_t *mem_aligned_1,\n                                 int32_t num_groups_5419, __global\n                                 unsigned char *mem_5651, __global\n                                 unsigned char *mem_5654, __global\n                                 unsigned char *mem_5663, __global\n                                 unsigned char *mem_5666)\n{\n    __local volatile char *restrict mem_5657 = mem_aligned_0;\n    __local volatile char *restrict mem_56",
            "60 = mem_aligned_1;\n    int32_t wave_sizze_5829;\n    int32_t group_sizze_5830;\n    bool thread_active_5831;\n    int32_t global_tid_5478;\n    int32_t local_tid_5479;\n    int32_t group_id_5480;\n    \n    global_tid_5478 = get_global_id(0);\n    local_tid_5479 = get_local_id(0);\n    group_sizze_5830 = get_local_size(0);\n    wave_sizze_5829 = LOCKSTEP_WIDTH;\n    group_id_5480 = get_group_id(0);\n    thread_active_5831 = 1;\n    \n    bool in_bounds_5481;\n    double x_5617;\n    double x_5619;\n    \n    if (thread_active_5831) {\n        in_bounds_5481 = slt32(local_tid_5479, num_groups_5419);\n        if (in_bounds_5481) {\n            double x_5482 = *(__global double *) &mem_5651[global_tid_5478 * 8];\n            \n            x_5617 = x_5482;\n        } else {\n            x_5617 = 0.0;\n        }\n        if (in_bounds_5481) {\n            double x_5484 = *(__global double *) &mem_5654[global_tid_5478 * 8];\n            \n            x_5619 = x_5484;\n        } else {\n            x_5619 = 0.0;\n        }\n    }\n    \n    double final_result_5489;\n    double final_result_5490;\n    \n    for (int32_t comb_iter_5832 = 0; comb_iter_5832 <\n         squot32(max_num_groups_5414 + max_num_groups_5414 - 1,\n                 max_num_groups_5414); comb_iter_5832++) {\n        int32_t combine_id_5488;\n        int32_t flat_comb_id_5833 = comb_iter_5832 * max_num_groups_5414 +\n                local_tid_5479;\n        \n        combine_id_5488 = flat_comb_id_5833;\n        if (slt32(combine_id_5488, max_num_groups_5414) && 1) {\n            *(__local double *) &mem_5657[combine_id_5488 * 8] = x_5617;\n            *(__local double *) &mem_5660[combine_id_5488 * 8] = x_5619;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5835;\n    int32_t skip_waves_5834;\n    double x_4913;\n    double x_4914;\n    double x_4915;\n    double x_4916;\n    int32_t my_index_5427;\n    int32_t other_index_5428;\n    \n    my_index_5427 = local_tid_5479;\n    offset_5835 = 0;\n    other_index_5428 = local_tid_5479 +",
            " offset_5835;\n    if (slt32(local_tid_5479, max_num_groups_5414)) {\n        x_4913 = *(__local double *) &mem_5657[(local_tid_5479 + offset_5835) *\n                                               8];\n        x_4914 = *(__local double *) &mem_5660[(local_tid_5479 + offset_5835) *\n                                               8];\n    }\n    offset_5835 = 1;\n    other_index_5428 = local_tid_5479 + offset_5835;\n    while (slt32(offset_5835, wave_sizze_5829)) {\n        if (slt32(other_index_5428, max_num_groups_5414) && ((local_tid_5479 -\n                                                              squot32(local_tid_5479,\n                                                                      wave_sizze_5829) *\n                                                              wave_sizze_5829) &\n                                                             (2 * offset_5835 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4915 = *(volatile __local\n                           double *) &mem_5657[(local_tid_5479 + offset_5835) *\n                                               8];\n                x_4916 = *(volatile __local\n                           double *) &mem_5660[(local_tid_5479 + offset_5835) *\n                                               8];\n            }\n            \n            double res_4917;\n            double res_4918;\n            \n            if (thread_active_5831) {\n                res_4917 = x_4913 + x_4915;\n                res_4918 = x_4914 + x_4916;\n            }\n            x_4913 = res_4917;\n            x_4914 = res_4918;\n            *(volatile __local double *) &mem_5657[local_tid_5479 * 8] = x_4913;\n            *(volatile __local double *) &mem_5660[local_tid_5479 * 8] = x_4914;\n        }\n        offset_5835 *= 2;\n        other_index_5428 = local_tid_5479 + offset_5835;\n    }\n    skip_waves_5834 = 1;\n    while (slt32(skip_waves_5834, squot32(max_num_groups_5414 +\n   ",
            "                                       wave_sizze_5829 - 1,\n                                          wave_sizze_5829))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5835 = skip_waves_5834 * wave_sizze_5829;\n        other_index_5428 = local_tid_5479 + offset_5835;\n        if (slt32(other_index_5428, max_num_groups_5414) && ((local_tid_5479 -\n                                                              squot32(local_tid_5479,\n                                                                      wave_sizze_5829) *\n                                                              wave_sizze_5829) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5479,\n                                                                      wave_sizze_5829) &\n                                                              (2 *\n                                                               skip_waves_5834 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4915 = *(__local double *) &mem_5657[(local_tid_5479 +\n                                                        offset_5835) * 8];\n                x_4916 = *(__local double *) &mem_5660[(local_tid_5479 +\n                                                        offset_5835) * 8];\n            }\n            \n            double res_4917;\n            double res_4918;\n            \n            if (thread_active_5831) {\n                res_4917 = x_4913 + x_4915;\n                res_4918 = x_4914 + x_4916;\n            }\n            x_4913 = res_4917;\n            x_4914 = res_4918;\n            *(__local double *) &mem_5657[local_tid_5479 * 8] = x_4913;\n            *(__local double *) &mem_5660[local_tid_5479 * 8] = x_4914;\n        }\n        skip_waves_5834 *= 2;\n    }\n    final_result_5489 = x_4913;\n    final_result_5490 = x_4914;\n    if (local_tid_5479 == ",
            "0) {\n        *(__global double *) &mem_5663[group_id_5480 * 8] = final_result_5489;\n    }\n    if (local_tid_5479 == 0) {\n        *(__global double *) &mem_5666[group_id_5480 * 8] = final_result_5490;\n    }\n}\n__kernel void reduce_kernel_5543(__local volatile int64_t *mem_aligned_0,\n                                 int32_t num_groups_5506, __global\n                                 unsigned char *mem_5636, __global\n                                 unsigned char *mem_5642)\n{\n    __local volatile char *restrict mem_5639 = mem_aligned_0;\n    int32_t wave_sizze_5848;\n    int32_t group_sizze_5849;\n    bool thread_active_5850;\n    int32_t global_tid_5543;\n    int32_t local_tid_5544;\n    int32_t group_id_5545;\n    \n    global_tid_5543 = get_global_id(0);\n    local_tid_5544 = get_local_id(0);\n    group_sizze_5849 = get_local_size(0);\n    wave_sizze_5848 = LOCKSTEP_WIDTH;\n    group_id_5545 = get_group_id(0);\n    thread_active_5850 = 1;\n    \n    bool in_bounds_5546;\n    double x_5615;\n    \n    if (thread_active_5850) {\n        in_bounds_5546 = slt32(local_tid_5544, num_groups_5506);\n        if (in_bounds_5546) {\n            double x_5547 = *(__global double *) &mem_5636[global_tid_5543 * 8];\n            \n            x_5615 = x_5547;\n        } else {\n            x_5615 = 0.0;\n        }\n    }\n    \n    double final_result_5551;\n    \n    for (int32_t comb_iter_5851 = 0; comb_iter_5851 <\n         squot32(max_num_groups_5501 + max_num_groups_5501 - 1,\n                 max_num_groups_5501); comb_iter_5851++) {\n        int32_t combine_id_5550;\n        int32_t flat_comb_id_5852 = comb_iter_5851 * max_num_groups_5501 +\n                local_tid_5544;\n        \n        combine_id_5550 = flat_comb_id_5852;\n        if (slt32(combine_id_5550, max_num_groups_5501) && 1) {\n            *(__local double *) &mem_5639[combine_id_5550 * 8] = x_5615;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5854;\n    int32_t skip_waves_5853;\n    double x_4931;\n    double x_4932;\n    in",
            "t32_t my_index_5513;\n    int32_t other_index_5514;\n    \n    my_index_5513 = local_tid_5544;\n    offset_5854 = 0;\n    other_index_5514 = local_tid_5544 + offset_5854;\n    if (slt32(local_tid_5544, max_num_groups_5501)) {\n        x_4931 = *(__local double *) &mem_5639[(local_tid_5544 + offset_5854) *\n                                               8];\n    }\n    offset_5854 = 1;\n    other_index_5514 = local_tid_5544 + offset_5854;\n    while (slt32(offset_5854, wave_sizze_5848)) {\n        if (slt32(other_index_5514, max_num_groups_5501) && ((local_tid_5544 -\n                                                              squot32(local_tid_5544,\n                                                                      wave_sizze_5848) *\n                                                              wave_sizze_5848) &\n                                                             (2 * offset_5854 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4932 = *(volatile __local\n                           double *) &mem_5639[(local_tid_5544 + offset_5854) *\n                                               8];\n            }\n            \n            double res_4933;\n            \n            if (thread_active_5850) {\n                res_4933 = x_4931 + x_4932;\n            }\n            x_4931 = res_4933;\n            *(volatile __local double *) &mem_5639[local_tid_5544 * 8] = x_4931;\n        }\n        offset_5854 *= 2;\n        other_index_5514 = local_tid_5544 + offset_5854;\n    }\n    skip_waves_5853 = 1;\n    while (slt32(skip_waves_5853, squot32(max_num_groups_5501 +\n                                          wave_sizze_5848 - 1,\n                                          wave_sizze_5848))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5854 = skip_waves_5853 * wave_sizze_5848;\n        other_index_5514 = local_tid_5544 + offset_5854;\n        if (slt32(other_index_5514, max_num_groups_5501) && ((lo",
            "cal_tid_5544 -\n                                                              squot32(local_tid_5544,\n                                                                      wave_sizze_5848) *\n                                                              wave_sizze_5848) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5544,\n                                                                      wave_sizze_5848) &\n                                                              (2 *\n                                                               skip_waves_5853 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4932 = *(__local double *) &mem_5639[(local_tid_5544 +\n                                                        offset_5854) * 8];\n            }\n            \n            double res_4933;\n            \n            if (thread_active_5850) {\n                res_4933 = x_4931 + x_4932;\n            }\n            x_4931 = res_4933;\n            *(__local double *) &mem_5639[local_tid_5544 * 8] = x_4931;\n        }\n        skip_waves_5853 *= 2;\n    }\n    final_result_5551 = x_4931;\n    if (local_tid_5544 == 0) {\n        *(__global double *) &mem_5642[group_id_5545 * 8] = final_result_5551;\n    }\n}\n__kernel void reduce_kernel_5606(__local volatile int64_t *mem_aligned_0,\n                                 int32_t num_groups_5567, __global\n                                 unsigned char *mem_5648, __global\n                                 unsigned char *mem_5654)\n{\n    __local volatile char *restrict mem_5651 = mem_aligned_0;\n    int32_t wave_sizze_5865;\n    int32_t group_sizze_5866;\n    bool thread_active_5867;\n    int32_t global_tid_5606;\n    int32_t local_tid_5607;\n    int32_t group_id_5608;\n    \n    global_tid_5606 = get_global_id(0);\n    local_tid_5607 = get_local_id(0);\n    group_sizze_58",
            "66 = get_local_size(0);\n    wave_sizze_5865 = LOCKSTEP_WIDTH;\n    group_id_5608 = get_group_id(0);\n    thread_active_5867 = 1;\n    \n    bool in_bounds_5609;\n    double x_5617;\n    \n    if (thread_active_5867) {\n        in_bounds_5609 = slt32(local_tid_5607, num_groups_5567);\n        if (in_bounds_5609) {\n            double x_5610 = *(__global double *) &mem_5648[global_tid_5606 * 8];\n            \n            x_5617 = x_5610;\n        } else {\n            x_5617 = 0.0;\n        }\n    }\n    \n    double final_result_5614;\n    \n    for (int32_t comb_iter_5868 = 0; comb_iter_5868 <\n         squot32(max_num_groups_5562 + max_num_groups_5562 - 1,\n                 max_num_groups_5562); comb_iter_5868++) {\n        int32_t combine_id_5613;\n        int32_t flat_comb_id_5869 = comb_iter_5868 * max_num_groups_5562 +\n                local_tid_5607;\n        \n        combine_id_5613 = flat_comb_id_5869;\n        if (slt32(combine_id_5613, max_num_groups_5562) && 1) {\n            *(__local double *) &mem_5651[combine_id_5613 * 8] = x_5617;\n        }\n    }\n    barrier(CLK_LOCAL_MEM_FENCE);\n    \n    int32_t offset_5871;\n    int32_t skip_waves_5870;\n    double x_4937;\n    double x_4938;\n    int32_t my_index_5574;\n    int32_t other_index_5575;\n    \n    my_index_5574 = local_tid_5607;\n    offset_5871 = 0;\n    other_index_5575 = local_tid_5607 + offset_5871;\n    if (slt32(local_tid_5607, max_num_groups_5562)) {\n        x_4937 = *(__local double *) &mem_5651[(local_tid_5607 + offset_5871) *\n                                               8];\n    }\n    offset_5871 = 1;\n    other_index_5575 = local_tid_5607 + offset_5871;\n    while (slt32(offset_5871, wave_sizze_5865)) {\n        if (slt32(other_index_5575, max_num_groups_5562) && ((local_tid_5607 -\n                                                              squot32(local_tid_5607,\n                                                                      wave_sizze_5865) *\n                                                              wave_sizze_586",
            "5) &\n                                                             (2 * offset_5871 -\n                                                              1)) == 0) {\n            // read array element\n            {\n                x_4938 = *(volatile __local\n                           double *) &mem_5651[(local_tid_5607 + offset_5871) *\n                                               8];\n            }\n            \n            double res_4939;\n            \n            if (thread_active_5867) {\n                res_4939 = x_4937 + x_4938;\n            }\n            x_4937 = res_4939;\n            *(volatile __local double *) &mem_5651[local_tid_5607 * 8] = x_4937;\n        }\n        offset_5871 *= 2;\n        other_index_5575 = local_tid_5607 + offset_5871;\n    }\n    skip_waves_5870 = 1;\n    while (slt32(skip_waves_5870, squot32(max_num_groups_5562 +\n                                          wave_sizze_5865 - 1,\n                                          wave_sizze_5865))) {\n        barrier(CLK_LOCAL_MEM_FENCE);\n        offset_5871 = skip_waves_5870 * wave_sizze_5865;\n        other_index_5575 = local_tid_5607 + offset_5871;\n        if (slt32(other_index_5575, max_num_groups_5562) && ((local_tid_5607 -\n                                                              squot32(local_tid_5607,\n                                                                      wave_sizze_5865) *\n                                                              wave_sizze_5865) ==\n                                                             0 &&\n                                                             (squot32(local_tid_5607,\n                                                                      wave_sizze_5865) &\n                                                              (2 *\n                                                               skip_waves_5870 -\n                                                               1)) == 0)) {\n            // read array element\n            {\n                x_4938 = *",
            "(__local double *) &mem_5651[(local_tid_5607 +\n                                                        offset_5871) * 8];\n            }\n            \n            double res_4939;\n            \n            if (thread_active_5867) {\n                res_4939 = x_4937 + x_4938;\n            }\n            x_4937 = res_4939;\n            *(__local double *) &mem_5651[local_tid_5607 * 8] = x_4937;\n        }\n        skip_waves_5870 *= 2;\n    }\n    final_result_5614 = x_4937;\n    if (local_tid_5607 == 0) {\n        *(__global double *) &mem_5654[group_id_5608 * 8] = final_result_5614;\n    }\n}\n",
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
static const char *size_names[] = {"group_size_4953", "max_num_groups_4955",
                                   "group_size_5014", "max_num_groups_5016",
                                   "group_size_5075", "max_num_groups_5077",
                                   "group_size_5136", "max_num_groups_5138",
                                   "group_size_5199", "max_num_groups_5201",
                                   "group_size_5262", "max_num_groups_5264",
                                   "group_size_5348", "max_num_groups_5350",
                                   "group_size_5411", "max_num_groups_5413",
                                   "group_size_5498", "max_num_groups_5500",
                                   "group_size_5559", "max_num_groups_5561"};
static const char *size_classes[] = {"group_size", "num_groups", "group_size",
                                     "num_groups", "group_size", "num_groups",
                                     "group_size", "num_groups", "group_size",
                                     "num_groups", "group_size", "num_groups",
                                     "group_size", "num_groups", "group_size",
                                     "num_groups", "group_size", "num_groups",
                                     "group_size", "num_groups"};
static const char *size_entry_points[] = {"sum", "sum", "mean", "mean",
                                          "variance", "variance", "variance",
                                          "variance", "skew", "skew", "skew",
                                          "skew", "kurtosis", "kurtosis",
                                          "kurtosis", "kurtosis", "stddev",
                                          "stddev", "stddev", "stddev"};
int futhark_gpu_get_num_sizes(void)
{
    return 20;
}
const char *futhark_gpu_get_size_name(int i)
{
    return size_names[i];
}
const char *futhark_gpu_get_size_class(int i)
{
    return size_classes[i];
}
const char *futhark_gpu_get_size_entry(int i)
{
    return size_entry_points[i];
}
struct sizes {
    size_t group_sizze_4953;
    size_t max_num_groups_4955;
    size_t group_sizze_5014;
    size_t max_num_groups_5016;
    size_t group_sizze_5075;
    size_t max_num_groups_5077;
    size_t group_sizze_5136;
    size_t max_num_groups_5138;
    size_t group_sizze_5199;
    size_t max_num_groups_5201;
    size_t group_sizze_5262;
    size_t max_num_groups_5264;
    size_t group_sizze_5348;
    size_t max_num_groups_5350;
    size_t group_sizze_5411;
    size_t max_num_groups_5413;
    size_t group_sizze_5498;
    size_t max_num_groups_5500;
    size_t group_sizze_5559;
    size_t max_num_groups_5561;
} ;
struct futhark_gpu_context_config {
    struct opencl_config opencl;
    size_t sizes[20];
} ;
struct futhark_gpu_context_config *futhark_gpu_context_config_new(void)
{
    struct futhark_gpu_context_config *cfg =
                                  malloc(sizeof(struct futhark_gpu_context_config));
    
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
    cfg->sizes[8] = 0;
    cfg->sizes[9] = 0;
    cfg->sizes[10] = 0;
    cfg->sizes[11] = 0;
    cfg->sizes[12] = 0;
    cfg->sizes[13] = 0;
    cfg->sizes[14] = 0;
    cfg->sizes[15] = 0;
    cfg->sizes[16] = 0;
    cfg->sizes[17] = 0;
    cfg->sizes[18] = 0;
    cfg->sizes[19] = 0;
    opencl_config_init(&cfg->opencl, 20, size_names, cfg->sizes, size_classes,
                       size_entry_points);
    cfg->opencl.transpose_block_dim = 16;
    return cfg;
}
void futhark_gpu_context_config_free(struct futhark_gpu_context_config *cfg)
{
    free(cfg);
}
void futhark_gpu_context_config_set_debugging(struct futhark_gpu_context_config *cfg,
                                          int flag)
{
    cfg->opencl.logging = cfg->opencl.debugging = flag;
}
void futhark_gpu_context_config_set_logging(struct futhark_gpu_context_config *cfg,
                                        int flag)
{
    cfg->opencl.logging = flag;
}
void futhark_gpu_context_config_set_device(struct futhark_gpu_context_config *cfg, const
                                       char *s)
{
    set_preferred_device(&cfg->opencl, s);
}
void futhark_gpu_context_config_set_platform(struct futhark_gpu_context_config *cfg,
                                         const char *s)
{
    set_preferred_platform(&cfg->opencl, s);
}
void futhark_gpu_context_config_dump_program_to(struct futhark_gpu_context_config *cfg,
                                            const char *path)
{
    cfg->opencl.dump_program_to = path;
}
void futhark_gpu_context_config_load_program_from(struct futhark_gpu_context_config *cfg,
                                              const char *path)
{
    cfg->opencl.load_program_from = path;
}
void futhark_gpu_context_config_set_default_group_size(struct futhark_gpu_context_config *cfg,
                                                   int size)
{
    cfg->opencl.default_group_size = size;
    cfg->opencl.default_group_size_changed = 1;
}
void futhark_gpu_context_config_set_default_num_groups(struct futhark_gpu_context_config *cfg,
                                                   int num)
{
    cfg->opencl.default_num_groups = num;
}
void futhark_gpu_context_config_set_default_tile_size(struct futhark_gpu_context_config *cfg,
                                                  int size)
{
    cfg->opencl.default_tile_size = size;
    cfg->opencl.default_tile_size_changed = 1;
}
void futhark_gpu_context_config_set_default_threshold(struct futhark_gpu_context_config *cfg,
                                                  int size)
{
    cfg->opencl.default_threshold = size;
}
int futhark_gpu_context_config_set_size(struct futhark_gpu_context_config *cfg, const
                                    char *size_name, size_t size_value)
{
    for (int i = 0; i < 20; i++) {
        if (strcmp(size_name, size_names[i]) == 0) {
            cfg->sizes[i] = size_value;
            return 0;
        }
    }
    return 1;
}
struct futhark_gpu_context {
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
    cl_kernel chunked_reduce_kernel_4970;
    int chunked_reduce_kernel_4970_total_runtime;
    int chunked_reduce_kernel_4970_runs;
    cl_kernel chunked_reduce_kernel_5031;
    int chunked_reduce_kernel_5031_total_runtime;
    int chunked_reduce_kernel_5031_runs;
    cl_kernel chunked_reduce_kernel_5092;
    int chunked_reduce_kernel_5092_total_runtime;
    int chunked_reduce_kernel_5092_runs;
    cl_kernel chunked_reduce_kernel_5153;
    int chunked_reduce_kernel_5153_total_runtime;
    int chunked_reduce_kernel_5153_runs;
    cl_kernel chunked_reduce_kernel_5216;
    int chunked_reduce_kernel_5216_total_runtime;
    int chunked_reduce_kernel_5216_runs;
    cl_kernel chunked_reduce_kernel_5280;
    int chunked_reduce_kernel_5280_total_runtime;
    int chunked_reduce_kernel_5280_runs;
    cl_kernel chunked_reduce_kernel_5365;
    int chunked_reduce_kernel_5365_total_runtime;
    int chunked_reduce_kernel_5365_runs;
    cl_kernel chunked_reduce_kernel_5429;
    int chunked_reduce_kernel_5429_total_runtime;
    int chunked_reduce_kernel_5429_runs;
    cl_kernel chunked_reduce_kernel_5515;
    int chunked_reduce_kernel_5515_total_runtime;
    int chunked_reduce_kernel_5515_runs;
    cl_kernel chunked_reduce_kernel_5576;
    int chunked_reduce_kernel_5576_total_runtime;
    int chunked_reduce_kernel_5576_runs;
    cl_kernel reduce_kernel_4998;
    int reduce_kernel_4998_total_runtime;
    int reduce_kernel_4998_runs;
    cl_kernel reduce_kernel_5059;
    int reduce_kernel_5059_total_runtime;
    int reduce_kernel_5059_runs;
    cl_kernel reduce_kernel_5120;
    int reduce_kernel_5120_total_runtime;
    int reduce_kernel_5120_runs;
    cl_kernel reduce_kernel_5183;
    int reduce_kernel_5183_total_runtime;
    int reduce_kernel_5183_runs;
    cl_kernel reduce_kernel_5244;
    int reduce_kernel_5244_total_runtime;
    int reduce_kernel_5244_runs;
    cl_kernel reduce_kernel_5328;
    int reduce_kernel_5328_total_runtime;
    int reduce_kernel_5328_runs;
    cl_kernel reduce_kernel_5393;
    int reduce_kernel_5393_total_runtime;
    int reduce_kernel_5393_runs;
    cl_kernel reduce_kernel_5478;
    int reduce_kernel_5478_total_runtime;
    int reduce_kernel_5478_runs;
    cl_kernel reduce_kernel_5543;
    int reduce_kernel_5543_total_runtime;
    int reduce_kernel_5543_runs;
    cl_kernel reduce_kernel_5606;
    int reduce_kernel_5606_total_runtime;
    int reduce_kernel_5606_runs;
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
static void init_context_early(struct futhark_gpu_context_config *cfg,
                               struct futhark_gpu_context *ctx)
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
    ctx->chunked_reduce_kernel_4970_total_runtime = 0;
    ctx->chunked_reduce_kernel_4970_runs = 0;
    ctx->chunked_reduce_kernel_5031_total_runtime = 0;
    ctx->chunked_reduce_kernel_5031_runs = 0;
    ctx->chunked_reduce_kernel_5092_total_runtime = 0;
    ctx->chunked_reduce_kernel_5092_runs = 0;
    ctx->chunked_reduce_kernel_5153_total_runtime = 0;
    ctx->chunked_reduce_kernel_5153_runs = 0;
    ctx->chunked_reduce_kernel_5216_total_runtime = 0;
    ctx->chunked_reduce_kernel_5216_runs = 0;
    ctx->chunked_reduce_kernel_5280_total_runtime = 0;
    ctx->chunked_reduce_kernel_5280_runs = 0;
    ctx->chunked_reduce_kernel_5365_total_runtime = 0;
    ctx->chunked_reduce_kernel_5365_runs = 0;
    ctx->chunked_reduce_kernel_5429_total_runtime = 0;
    ctx->chunked_reduce_kernel_5429_runs = 0;
    ctx->chunked_reduce_kernel_5515_total_runtime = 0;
    ctx->chunked_reduce_kernel_5515_runs = 0;
    ctx->chunked_reduce_kernel_5576_total_runtime = 0;
    ctx->chunked_reduce_kernel_5576_runs = 0;
    ctx->reduce_kernel_4998_total_runtime = 0;
    ctx->reduce_kernel_4998_runs = 0;
    ctx->reduce_kernel_5059_total_runtime = 0;
    ctx->reduce_kernel_5059_runs = 0;
    ctx->reduce_kernel_5120_total_runtime = 0;
    ctx->reduce_kernel_5120_runs = 0;
    ctx->reduce_kernel_5183_total_runtime = 0;
    ctx->reduce_kernel_5183_runs = 0;
    ctx->reduce_kernel_5244_total_runtime = 0;
    ctx->reduce_kernel_5244_runs = 0;
    ctx->reduce_kernel_5328_total_runtime = 0;
    ctx->reduce_kernel_5328_runs = 0;
    ctx->reduce_kernel_5393_total_runtime = 0;
    ctx->reduce_kernel_5393_runs = 0;
    ctx->reduce_kernel_5478_total_runtime = 0;
    ctx->reduce_kernel_5478_runs = 0;
    ctx->reduce_kernel_5543_total_runtime = 0;
    ctx->reduce_kernel_5543_runs = 0;
    ctx->reduce_kernel_5606_total_runtime = 0;
    ctx->reduce_kernel_5606_runs = 0;
}
static void init_context_late(struct futhark_gpu_context_config *cfg,
                              struct futhark_gpu_context *ctx, cl_program prog)
{
    cl_int error;
    
    {
        ctx->chunked_reduce_kernel_4970 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_4970",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_4970");
    }
    {
        ctx->chunked_reduce_kernel_5031 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5031",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5031");
    }
    {
        ctx->chunked_reduce_kernel_5092 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5092",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5092");
    }
    {
        ctx->chunked_reduce_kernel_5153 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5153",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5153");
    }
    {
        ctx->chunked_reduce_kernel_5216 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5216",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5216");
    }
    {
        ctx->chunked_reduce_kernel_5280 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5280",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5280");
    }
    {
        ctx->chunked_reduce_kernel_5365 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5365",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5365");
    }
    {
        ctx->chunked_reduce_kernel_5429 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5429",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5429");
    }
    {
        ctx->chunked_reduce_kernel_5515 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5515",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5515");
    }
    {
        ctx->chunked_reduce_kernel_5576 = clCreateKernel(prog,
                                                         "chunked_reduce_kernel_5576",
                                                         &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n",
                    "chunked_reduce_kernel_5576");
    }
    {
        ctx->reduce_kernel_4998 = clCreateKernel(prog, "reduce_kernel_4998",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_4998");
    }
    {
        ctx->reduce_kernel_5059 = clCreateKernel(prog, "reduce_kernel_5059",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5059");
    }
    {
        ctx->reduce_kernel_5120 = clCreateKernel(prog, "reduce_kernel_5120",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5120");
    }
    {
        ctx->reduce_kernel_5183 = clCreateKernel(prog, "reduce_kernel_5183",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5183");
    }
    {
        ctx->reduce_kernel_5244 = clCreateKernel(prog, "reduce_kernel_5244",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5244");
    }
    {
        ctx->reduce_kernel_5328 = clCreateKernel(prog, "reduce_kernel_5328",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5328");
    }
    {
        ctx->reduce_kernel_5393 = clCreateKernel(prog, "reduce_kernel_5393",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5393");
    }
    {
        ctx->reduce_kernel_5478 = clCreateKernel(prog, "reduce_kernel_5478",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5478");
    }
    {
        ctx->reduce_kernel_5543 = clCreateKernel(prog, "reduce_kernel_5543",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5543");
    }
    {
        ctx->reduce_kernel_5606 = clCreateKernel(prog, "reduce_kernel_5606",
                                                 &error);
        assert(error == 0);
        if (ctx->debugging)
            fprintf(stderr, "Created kernel %s.\n", "reduce_kernel_5606");
    }
    ctx->sizes.group_sizze_4953 = cfg->sizes[0];
    ctx->sizes.max_num_groups_4955 = cfg->sizes[1];
    ctx->sizes.group_sizze_5014 = cfg->sizes[2];
    ctx->sizes.max_num_groups_5016 = cfg->sizes[3];
    ctx->sizes.group_sizze_5075 = cfg->sizes[4];
    ctx->sizes.max_num_groups_5077 = cfg->sizes[5];
    ctx->sizes.group_sizze_5136 = cfg->sizes[6];
    ctx->sizes.max_num_groups_5138 = cfg->sizes[7];
    ctx->sizes.group_sizze_5199 = cfg->sizes[8];
    ctx->sizes.max_num_groups_5201 = cfg->sizes[9];
    ctx->sizes.group_sizze_5262 = cfg->sizes[10];
    ctx->sizes.max_num_groups_5264 = cfg->sizes[11];
    ctx->sizes.group_sizze_5348 = cfg->sizes[12];
    ctx->sizes.max_num_groups_5350 = cfg->sizes[13];
    ctx->sizes.group_sizze_5411 = cfg->sizes[14];
    ctx->sizes.max_num_groups_5413 = cfg->sizes[15];
    ctx->sizes.group_sizze_5498 = cfg->sizes[16];
    ctx->sizes.max_num_groups_5500 = cfg->sizes[17];
    ctx->sizes.group_sizze_5559 = cfg->sizes[18];
    ctx->sizes.max_num_groups_5561 = cfg->sizes[19];
}
struct futhark_gpu_context *futhark_gpu_context_new(struct futhark_gpu_context_config *cfg)
{
    struct futhark_gpu_context *ctx = malloc(sizeof(struct futhark_gpu_context));
    
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
struct futhark_gpu_context *futhark_gpu_context_new_with_command_queue(struct futhark_gpu_context_config *cfg,
                                                               cl_command_queue queue)
{
    struct futhark_gpu_context *ctx = malloc(sizeof(struct futhark_gpu_context));
    
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
void futhark_gpu_context_free(struct futhark_gpu_context *ctx)
{
    free_lock(&ctx->lock);
    free(ctx);
}
int futhark_gpu_context_sync(struct futhark_gpu_context *ctx)
{
    OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
    return 0;
}
char *futhark_gpu_context_get_error(struct futhark_gpu_context *ctx)
{
    char *error = ctx->error;
    
    ctx->error = NULL;
    return error;
}
int futhark_gpu_context_clear_caches(struct futhark_gpu_context *ctx)
{
    OPENCL_SUCCEED(opencl_free_all(&ctx->opencl));
    return 0;
}
cl_command_queue futhark_gpu_context_get_command_queue(struct futhark_gpu_context *ctx)
{
    return ctx->opencl.queue;
}
static void memblock_unref_device(struct futhark_gpu_context *ctx,
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
static void memblock_alloc_device(struct futhark_gpu_context *ctx,
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
static void memblock_set_device(struct futhark_gpu_context *ctx,
                                struct memblock_device *lhs,
                                struct memblock_device *rhs, const
                                char *lhs_desc)
{
    memblock_unref_device(ctx, lhs, lhs_desc);
    (*rhs->references)++;
    *lhs = *rhs;
}
static void memblock_unref_local(struct futhark_gpu_context *ctx,
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
static void memblock_alloc_local(struct futhark_gpu_context *ctx,
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
static void memblock_set_local(struct futhark_gpu_context *ctx,
                               struct memblock_local *lhs,
                               struct memblock_local *rhs, const char *lhs_desc)
{
    memblock_unref_local(ctx, lhs, lhs_desc);
    (*rhs->references)++;
    *lhs = *rhs;
}
static void memblock_unref(struct futhark_gpu_context *ctx, struct memblock *block,
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
static void memblock_alloc(struct futhark_gpu_context *ctx, struct memblock *block,
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
static void memblock_set(struct futhark_gpu_context *ctx, struct memblock *lhs,
                         struct memblock *rhs, const char *lhs_desc)
{
    memblock_unref(ctx, lhs, lhs_desc);
    (*rhs->references)++;
    *lhs = *rhs;
}
void futhark_gpu_debugging_report(struct futhark_gpu_context *ctx)
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
                "Kernel chunked_reduce_kernel_4970 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_4970_runs,
                (long) ctx->chunked_reduce_kernel_4970_total_runtime /
                (ctx->chunked_reduce_kernel_4970_runs !=
                 0 ? ctx->chunked_reduce_kernel_4970_runs : 1),
                (long) ctx->chunked_reduce_kernel_4970_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_4970_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_4970_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5031 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5031_runs,
                (long) ctx->chunked_reduce_kernel_5031_total_runtime /
                (ctx->chunked_reduce_kernel_5031_runs !=
                 0 ? ctx->chunked_reduce_kernel_5031_runs : 1),
                (long) ctx->chunked_reduce_kernel_5031_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5031_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5031_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5092 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5092_runs,
                (long) ctx->chunked_reduce_kernel_5092_total_runtime /
                (ctx->chunked_reduce_kernel_5092_runs !=
                 0 ? ctx->chunked_reduce_kernel_5092_runs : 1),
                (long) ctx->chunked_reduce_kernel_5092_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5092_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5092_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5153 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5153_runs,
                (long) ctx->chunked_reduce_kernel_5153_total_runtime /
                (ctx->chunked_reduce_kernel_5153_runs !=
                 0 ? ctx->chunked_reduce_kernel_5153_runs : 1),
                (long) ctx->chunked_reduce_kernel_5153_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5153_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5153_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5216 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5216_runs,
                (long) ctx->chunked_reduce_kernel_5216_total_runtime /
                (ctx->chunked_reduce_kernel_5216_runs !=
                 0 ? ctx->chunked_reduce_kernel_5216_runs : 1),
                (long) ctx->chunked_reduce_kernel_5216_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5216_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5216_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5280 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5280_runs,
                (long) ctx->chunked_reduce_kernel_5280_total_runtime /
                (ctx->chunked_reduce_kernel_5280_runs !=
                 0 ? ctx->chunked_reduce_kernel_5280_runs : 1),
                (long) ctx->chunked_reduce_kernel_5280_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5280_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5280_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5365 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5365_runs,
                (long) ctx->chunked_reduce_kernel_5365_total_runtime /
                (ctx->chunked_reduce_kernel_5365_runs !=
                 0 ? ctx->chunked_reduce_kernel_5365_runs : 1),
                (long) ctx->chunked_reduce_kernel_5365_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5365_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5365_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5429 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5429_runs,
                (long) ctx->chunked_reduce_kernel_5429_total_runtime /
                (ctx->chunked_reduce_kernel_5429_runs !=
                 0 ? ctx->chunked_reduce_kernel_5429_runs : 1),
                (long) ctx->chunked_reduce_kernel_5429_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5429_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5429_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5515 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5515_runs,
                (long) ctx->chunked_reduce_kernel_5515_total_runtime /
                (ctx->chunked_reduce_kernel_5515_runs !=
                 0 ? ctx->chunked_reduce_kernel_5515_runs : 1),
                (long) ctx->chunked_reduce_kernel_5515_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5515_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5515_runs;
        fprintf(stderr,
                "Kernel chunked_reduce_kernel_5576 executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->chunked_reduce_kernel_5576_runs,
                (long) ctx->chunked_reduce_kernel_5576_total_runtime /
                (ctx->chunked_reduce_kernel_5576_runs !=
                 0 ? ctx->chunked_reduce_kernel_5576_runs : 1),
                (long) ctx->chunked_reduce_kernel_5576_total_runtime);
        ctx->total_runtime += ctx->chunked_reduce_kernel_5576_total_runtime;
        ctx->total_runs += ctx->chunked_reduce_kernel_5576_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_4998         executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_4998_runs,
                (long) ctx->reduce_kernel_4998_total_runtime /
                (ctx->reduce_kernel_4998_runs !=
                 0 ? ctx->reduce_kernel_4998_runs : 1),
                (long) ctx->reduce_kernel_4998_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_4998_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_4998_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5059         executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5059_runs,
                (long) ctx->reduce_kernel_5059_total_runtime /
                (ctx->reduce_kernel_5059_runs !=
                 0 ? ctx->reduce_kernel_5059_runs : 1),
                (long) ctx->reduce_kernel_5059_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5059_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5059_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5120         executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5120_runs,
                (long) ctx->reduce_kernel_5120_total_runtime /
                (ctx->reduce_kernel_5120_runs !=
                 0 ? ctx->reduce_kernel_5120_runs : 1),
                (long) ctx->reduce_kernel_5120_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5120_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5120_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5183         executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5183_runs,
                (long) ctx->reduce_kernel_5183_total_runtime /
                (ctx->reduce_kernel_5183_runs !=
                 0 ? ctx->reduce_kernel_5183_runs : 1),
                (long) ctx->reduce_kernel_5183_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5183_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5183_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5244         executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5244_runs,
                (long) ctx->reduce_kernel_5244_total_runtime /
                (ctx->reduce_kernel_5244_runs !=
                 0 ? ctx->reduce_kernel_5244_runs : 1),
                (long) ctx->reduce_kernel_5244_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5244_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5244_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5328         executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5328_runs,
                (long) ctx->reduce_kernel_5328_total_runtime /
                (ctx->reduce_kernel_5328_runs !=
                 0 ? ctx->reduce_kernel_5328_runs : 1),
                (long) ctx->reduce_kernel_5328_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5328_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5328_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5393         executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5393_runs,
                (long) ctx->reduce_kernel_5393_total_runtime /
                (ctx->reduce_kernel_5393_runs !=
                 0 ? ctx->reduce_kernel_5393_runs : 1),
                (long) ctx->reduce_kernel_5393_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5393_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5393_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5478         executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5478_runs,
                (long) ctx->reduce_kernel_5478_total_runtime /
                (ctx->reduce_kernel_5478_runs !=
                 0 ? ctx->reduce_kernel_5478_runs : 1),
                (long) ctx->reduce_kernel_5478_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5478_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5478_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5543         executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5543_runs,
                (long) ctx->reduce_kernel_5543_total_runtime /
                (ctx->reduce_kernel_5543_runs !=
                 0 ? ctx->reduce_kernel_5543_runs : 1),
                (long) ctx->reduce_kernel_5543_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5543_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5543_runs;
        fprintf(stderr,
                "Kernel reduce_kernel_5606         executed %6d times, with average runtime: %6ldus\tand total runtime: %6ldus\n",
                ctx->reduce_kernel_5606_runs,
                (long) ctx->reduce_kernel_5606_total_runtime /
                (ctx->reduce_kernel_5606_runs !=
                 0 ? ctx->reduce_kernel_5606_runs : 1),
                (long) ctx->reduce_kernel_5606_total_runtime);
        ctx->total_runtime += ctx->reduce_kernel_5606_total_runtime;
        ctx->total_runs += ctx->reduce_kernel_5606_runs;
        if (ctx->debugging)
            fprintf(stderr, "Ran %d kernels with cumulative runtime: %6ldus\n",
                    ctx->total_runs, ctx->total_runtime);
    }
}
static int futrts_sum(struct futhark_gpu_context *ctx, double *out_scalar_out_5873,
                      int64_t col_mem_sizze_5629,
                      struct memblock_device col_mem_5630, int32_t sizze_4841);
static int futrts_mean(struct futhark_gpu_context *ctx, double *out_scalar_out_5885,
                       int64_t col_mem_sizze_5629,
                       struct memblock_device col_mem_5630, int32_t sizze_4848);
static int futrts_variance(struct futhark_gpu_context *ctx,
                           double *out_scalar_out_5897,
                           int64_t values_mem_sizze_5629,
                           struct memblock_device values_mem_5630,
                           int32_t sizze_4857);
static int futrts_skew(struct futhark_gpu_context *ctx, double *out_scalar_out_5920,
                       int64_t values_mem_sizze_5629,
                       struct memblock_device values_mem_5630,
                       int32_t sizze_4875);
static int futrts_kurtosis(struct futhark_gpu_context *ctx,
                           double *out_scalar_out_5944,
                           int64_t values_mem_sizze_5629,
                           struct memblock_device values_mem_5630,
                           int32_t sizze_4902);
static int futrts_stddev(struct futhark_gpu_context *ctx,
                         double *out_scalar_out_5968,
                         int64_t values_mem_sizze_5629,
                         struct memblock_device values_mem_5630,
                         int32_t sizze_4927);
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
static int futrts_sum(struct futhark_gpu_context *ctx, double *out_scalar_out_5873,
                      int64_t col_mem_sizze_5629,
                      struct memblock_device col_mem_5630, int32_t sizze_4841)
{
    double scalar_out_5687;
    int32_t group_sizze_4954;
    
    group_sizze_4954 = ctx->sizes.group_sizze_4953;
    
    int32_t max_num_groups_4956;
    
    max_num_groups_4956 = ctx->sizes.max_num_groups_4955;
    
    int32_t y_4957 = group_sizze_4954 - 1;
    int32_t x_4958 = sizze_4841 + y_4957;
    int32_t w_div_group_sizze_4959 = squot32(x_4958, group_sizze_4954);
    int32_t num_groups_maybe_zzero_4960 = smin32(max_num_groups_4956,
                                                 w_div_group_sizze_4959);
    int32_t num_groups_4961 = smax32(1, num_groups_maybe_zzero_4960);
    int32_t num_threads_4962 = group_sizze_4954 * num_groups_4961;
    int32_t y_4963 = num_threads_4962 - 1;
    int32_t x_4964 = sizze_4841 + y_4963;
    int32_t per_thread_elements_4965 = squot32(x_4964, num_threads_4962);
    int64_t binop_x_5635 = sext_i32_i64(num_groups_4961);
    int64_t bytes_5634 = 8 * binop_x_5635;
    struct memblock_device mem_5636;
    
    mem_5636.references = NULL;
    memblock_alloc_device(ctx, &mem_5636, bytes_5634, "mem_5636");
    
    int64_t binop_x_5632 = sext_i32_i64(group_sizze_4954);
    int64_t bytes_5631 = 8 * binop_x_5632;
    struct memblock_local mem_5633;
    
    mem_5633.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4841);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4970, 0,
                                  bytes_5631, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4970, 1,
                                  sizeof(sizze_4841), &sizze_4841));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4970, 2,
                                  sizeof(num_threads_4962), &num_threads_4962));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4970, 3,
                                  sizeof(per_thread_elements_4965),
                                  &per_thread_elements_4965));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4970, 4,
                                  sizeof(col_mem_5630.mem), &col_mem_5630.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_4970, 5,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    if (1 * (num_groups_4961 * group_sizze_4954) != 0) {
        const size_t global_work_sizze_5874[1] = {num_groups_4961 *
                     group_sizze_4954};
        const size_t local_work_sizze_5878[1] = {group_sizze_4954};
        int64_t time_start_5875 = 0, time_end_5876 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_4970");
            fprintf(stderr, "%zu", global_work_sizze_5874[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5878[0]);
            fprintf(stderr, "].\n");
            time_start_5875 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_4970,
                                              1, NULL, global_work_sizze_5874,
                                              local_work_sizze_5878, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5876 = get_wall_time();
            
            long time_diff_5877 = time_end_5876 - time_start_5875;
            
            ctx->chunked_reduce_kernel_4970_total_runtime += time_diff_5877;
            ctx->chunked_reduce_kernel_4970_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_4970", time_diff_5877);
        }
    }
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    
    struct memblock_device mem_5642;
    
    mem_5642.references = NULL;
    memblock_alloc_device(ctx, &mem_5642, 8, "mem_5642");
    
    int64_t binop_x_5638 = sext_i32_i64(max_num_groups_4956);
    int64_t bytes_5637 = 8 * binop_x_5638;
    struct memblock_local mem_5639;
    
    mem_5639.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_4998, 0, bytes_5637,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_4998, 1,
                                  sizeof(num_groups_4961), &num_groups_4961));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_4998, 2,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_4998, 3,
                                  sizeof(mem_5642.mem), &mem_5642.mem));
    if (1 * max_num_groups_4956 != 0) {
        const size_t global_work_sizze_5879[1] = {max_num_groups_4956};
        const size_t local_work_sizze_5883[1] = {max_num_groups_4956};
        int64_t time_start_5880 = 0, time_end_5881 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_4998");
            fprintf(stderr, "%zu", global_work_sizze_5879[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5883[0]);
            fprintf(stderr, "].\n");
            time_start_5880 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_4998, 1, NULL,
                                              global_work_sizze_5879,
                                              local_work_sizze_5883, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5881 = get_wall_time();
            
            long time_diff_5882 = time_end_5881 - time_start_5880;
            
            ctx->reduce_kernel_4998_total_runtime += time_diff_5882;
            ctx->reduce_kernel_4998_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_4998",
                    time_diff_5882);
        }
    }
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    
    double read_res_5884;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5642.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5884, 0,
                                       NULL, NULL));
    
    double res_4843 = read_res_5884;
    
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    scalar_out_5687 = res_4843;
    *out_scalar_out_5873 = scalar_out_5687;
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    return 0;
}
static int futrts_mean(struct futhark_gpu_context *ctx, double *out_scalar_out_5885,
                       int64_t col_mem_sizze_5629,
                       struct memblock_device col_mem_5630, int32_t sizze_4848)
{
    double scalar_out_5705;
    int32_t group_sizze_5015;
    
    group_sizze_5015 = ctx->sizes.group_sizze_5014;
    
    int32_t max_num_groups_5017;
    
    max_num_groups_5017 = ctx->sizes.max_num_groups_5016;
    
    int32_t y_5018 = group_sizze_5015 - 1;
    int32_t x_5019 = sizze_4848 + y_5018;
    int32_t w_div_group_sizze_5020 = squot32(x_5019, group_sizze_5015);
    int32_t num_groups_maybe_zzero_5021 = smin32(max_num_groups_5017,
                                                 w_div_group_sizze_5020);
    int32_t num_groups_5022 = smax32(1, num_groups_maybe_zzero_5021);
    int32_t num_threads_5023 = group_sizze_5015 * num_groups_5022;
    int32_t y_5024 = num_threads_5023 - 1;
    int32_t x_5025 = sizze_4848 + y_5024;
    int32_t per_thread_elements_5026 = squot32(x_5025, num_threads_5023);
    int64_t binop_x_5635 = sext_i32_i64(num_groups_5022);
    int64_t bytes_5634 = 8 * binop_x_5635;
    struct memblock_device mem_5636;
    
    mem_5636.references = NULL;
    memblock_alloc_device(ctx, &mem_5636, bytes_5634, "mem_5636");
    
    int64_t binop_x_5632 = sext_i32_i64(group_sizze_5015);
    int64_t bytes_5631 = 8 * binop_x_5632;
    struct memblock_local mem_5633;
    
    mem_5633.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4848);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5031, 0,
                                  bytes_5631, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5031, 1,
                                  sizeof(sizze_4848), &sizze_4848));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5031, 2,
                                  sizeof(num_threads_5023), &num_threads_5023));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5031, 3,
                                  sizeof(per_thread_elements_5026),
                                  &per_thread_elements_5026));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5031, 4,
                                  sizeof(col_mem_5630.mem), &col_mem_5630.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5031, 5,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    if (1 * (num_groups_5022 * group_sizze_5015) != 0) {
        const size_t global_work_sizze_5886[1] = {num_groups_5022 *
                     group_sizze_5015};
        const size_t local_work_sizze_5890[1] = {group_sizze_5015};
        int64_t time_start_5887 = 0, time_end_5888 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5031");
            fprintf(stderr, "%zu", global_work_sizze_5886[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5890[0]);
            fprintf(stderr, "].\n");
            time_start_5887 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5031,
                                              1, NULL, global_work_sizze_5886,
                                              local_work_sizze_5890, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5888 = get_wall_time();
            
            long time_diff_5889 = time_end_5888 - time_start_5887;
            
            ctx->chunked_reduce_kernel_5031_total_runtime += time_diff_5889;
            ctx->chunked_reduce_kernel_5031_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5031", time_diff_5889);
        }
    }
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    
    struct memblock_device mem_5642;
    
    mem_5642.references = NULL;
    memblock_alloc_device(ctx, &mem_5642, 8, "mem_5642");
    
    int64_t binop_x_5638 = sext_i32_i64(max_num_groups_5017);
    int64_t bytes_5637 = 8 * binop_x_5638;
    struct memblock_local mem_5639;
    
    mem_5639.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5059, 0, bytes_5637,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5059, 1,
                                  sizeof(num_groups_5022), &num_groups_5022));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5059, 2,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5059, 3,
                                  sizeof(mem_5642.mem), &mem_5642.mem));
    if (1 * max_num_groups_5017 != 0) {
        const size_t global_work_sizze_5891[1] = {max_num_groups_5017};
        const size_t local_work_sizze_5895[1] = {max_num_groups_5017};
        int64_t time_start_5892 = 0, time_end_5893 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5059");
            fprintf(stderr, "%zu", global_work_sizze_5891[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5895[0]);
            fprintf(stderr, "].\n");
            time_start_5892 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5059, 1, NULL,
                                              global_work_sizze_5891,
                                              local_work_sizze_5895, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5893 = get_wall_time();
            
            long time_diff_5894 = time_end_5893 - time_start_5892;
            
            ctx->reduce_kernel_5059_total_runtime += time_diff_5894;
            ctx->reduce_kernel_5059_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5059",
                    time_diff_5894);
        }
    }
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    
    double read_res_5896;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5642.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5896, 0,
                                       NULL, NULL));
    
    double res_4850 = read_res_5896;
    
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    
    double res_4855 = sitofp_i32_f64(sizze_4848);
    double res_4856 = res_4850 / res_4855;
    
    scalar_out_5705 = res_4856;
    *out_scalar_out_5885 = scalar_out_5705;
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    return 0;
}
static int futrts_variance(struct futhark_gpu_context *ctx,
                           double *out_scalar_out_5897,
                           int64_t values_mem_sizze_5629,
                           struct memblock_device values_mem_5630,
                           int32_t sizze_4857)
{
    double scalar_out_5723;
    double res_4859 = sitofp_i32_f64(sizze_4857);
    int32_t group_sizze_5076;
    
    group_sizze_5076 = ctx->sizes.group_sizze_5075;
    
    int32_t max_num_groups_5078;
    
    max_num_groups_5078 = ctx->sizes.max_num_groups_5077;
    
    int32_t y_5079 = group_sizze_5076 - 1;
    int32_t x_5080 = sizze_4857 + y_5079;
    int32_t w_div_group_sizze_5081 = squot32(x_5080, group_sizze_5076);
    int32_t num_groups_maybe_zzero_5082 = smin32(max_num_groups_5078,
                                                 w_div_group_sizze_5081);
    int32_t num_groups_5083 = smax32(1, num_groups_maybe_zzero_5082);
    int32_t num_threads_5084 = group_sizze_5076 * num_groups_5083;
    int32_t y_5085 = num_threads_5084 - 1;
    int32_t x_5086 = sizze_4857 + y_5085;
    int32_t per_thread_elements_5087 = squot32(x_5086, num_threads_5084);
    int64_t binop_x_5635 = sext_i32_i64(num_groups_5083);
    int64_t bytes_5634 = 8 * binop_x_5635;
    struct memblock_device mem_5636;
    
    mem_5636.references = NULL;
    memblock_alloc_device(ctx, &mem_5636, bytes_5634, "mem_5636");
    
    int64_t binop_x_5632 = sext_i32_i64(group_sizze_5076);
    int64_t bytes_5631 = 8 * binop_x_5632;
    struct memblock_local mem_5633;
    
    mem_5633.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4857);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5092, 0,
                                  bytes_5631, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5092, 1,
                                  sizeof(sizze_4857), &sizze_4857));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5092, 2,
                                  sizeof(num_threads_5084), &num_threads_5084));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5092, 3,
                                  sizeof(per_thread_elements_5087),
                                  &per_thread_elements_5087));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5092, 4,
                                  sizeof(values_mem_5630.mem),
                                  &values_mem_5630.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5092, 5,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    if (1 * (num_groups_5083 * group_sizze_5076) != 0) {
        const size_t global_work_sizze_5898[1] = {num_groups_5083 *
                     group_sizze_5076};
        const size_t local_work_sizze_5902[1] = {group_sizze_5076};
        int64_t time_start_5899 = 0, time_end_5900 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5092");
            fprintf(stderr, "%zu", global_work_sizze_5898[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5902[0]);
            fprintf(stderr, "].\n");
            time_start_5899 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5092,
                                              1, NULL, global_work_sizze_5898,
                                              local_work_sizze_5902, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5900 = get_wall_time();
            
            long time_diff_5901 = time_end_5900 - time_start_5899;
            
            ctx->chunked_reduce_kernel_5092_total_runtime += time_diff_5901;
            ctx->chunked_reduce_kernel_5092_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5092", time_diff_5901);
        }
    }
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    
    struct memblock_device mem_5642;
    
    mem_5642.references = NULL;
    memblock_alloc_device(ctx, &mem_5642, 8, "mem_5642");
    
    int64_t binop_x_5638 = sext_i32_i64(max_num_groups_5078);
    int64_t bytes_5637 = 8 * binop_x_5638;
    struct memblock_local mem_5639;
    
    mem_5639.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5120, 0, bytes_5637,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5120, 1,
                                  sizeof(num_groups_5083), &num_groups_5083));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5120, 2,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5120, 3,
                                  sizeof(mem_5642.mem), &mem_5642.mem));
    if (1 * max_num_groups_5078 != 0) {
        const size_t global_work_sizze_5903[1] = {max_num_groups_5078};
        const size_t local_work_sizze_5907[1] = {max_num_groups_5078};
        int64_t time_start_5904 = 0, time_end_5905 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5120");
            fprintf(stderr, "%zu", global_work_sizze_5903[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5907[0]);
            fprintf(stderr, "].\n");
            time_start_5904 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5120, 1, NULL,
                                              global_work_sizze_5903,
                                              local_work_sizze_5907, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5905 = get_wall_time();
            
            long time_diff_5906 = time_end_5905 - time_start_5904;
            
            ctx->reduce_kernel_5120_total_runtime += time_diff_5906;
            ctx->reduce_kernel_5120_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5120",
                    time_diff_5906);
        }
    }
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    
    double read_res_5908;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5642.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5908, 0,
                                       NULL, NULL));
    
    double res_4860 = read_res_5908;
    
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    
    double res_4865 = res_4860 / res_4859;
    int32_t group_sizze_5137;
    
    group_sizze_5137 = ctx->sizes.group_sizze_5136;
    
    int32_t max_num_groups_5139;
    
    max_num_groups_5139 = ctx->sizes.max_num_groups_5138;
    
    int32_t y_5140 = group_sizze_5137 - 1;
    int32_t x_5141 = sizze_4857 + y_5140;
    int32_t w_div_group_sizze_5142 = squot32(x_5141, group_sizze_5137);
    int32_t num_groups_maybe_zzero_5143 = smin32(max_num_groups_5139,
                                                 w_div_group_sizze_5142);
    int32_t num_groups_5144 = smax32(1, num_groups_maybe_zzero_5143);
    int32_t num_threads_5145 = group_sizze_5137 * num_groups_5144;
    int32_t y_5146 = num_threads_5145 - 1;
    int32_t x_5147 = sizze_4857 + y_5146;
    int32_t per_thread_elements_5148 = squot32(x_5147, num_threads_5145);
    int64_t binop_x_5647 = sext_i32_i64(num_groups_5144);
    int64_t bytes_5646 = 8 * binop_x_5647;
    struct memblock_device mem_5648;
    
    mem_5648.references = NULL;
    memblock_alloc_device(ctx, &mem_5648, bytes_5646, "mem_5648");
    
    int64_t binop_x_5644 = sext_i32_i64(group_sizze_5137);
    int64_t bytes_5643 = 8 * binop_x_5644;
    struct memblock_local mem_5645;
    
    mem_5645.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4857);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5153, 0,
                                  bytes_5643, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5153, 1,
                                  sizeof(sizze_4857), &sizze_4857));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5153, 2,
                                  sizeof(res_4865), &res_4865));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5153, 3,
                                  sizeof(num_threads_5145), &num_threads_5145));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5153, 4,
                                  sizeof(per_thread_elements_5148),
                                  &per_thread_elements_5148));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5153, 5,
                                  sizeof(values_mem_5630.mem),
                                  &values_mem_5630.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5153, 6,
                                  sizeof(mem_5648.mem), &mem_5648.mem));
    if (1 * (num_groups_5144 * group_sizze_5137) != 0) {
        const size_t global_work_sizze_5909[1] = {num_groups_5144 *
                     group_sizze_5137};
        const size_t local_work_sizze_5913[1] = {group_sizze_5137};
        int64_t time_start_5910 = 0, time_end_5911 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5153");
            fprintf(stderr, "%zu", global_work_sizze_5909[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5913[0]);
            fprintf(stderr, "].\n");
            time_start_5910 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5153,
                                              1, NULL, global_work_sizze_5909,
                                              local_work_sizze_5913, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5911 = get_wall_time();
            
            long time_diff_5912 = time_end_5911 - time_start_5910;
            
            ctx->chunked_reduce_kernel_5153_total_runtime += time_diff_5912;
            ctx->chunked_reduce_kernel_5153_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5153", time_diff_5912);
        }
    }
    memblock_unref_local(ctx, &mem_5645, "mem_5645");
    
    struct memblock_device mem_5654;
    
    mem_5654.references = NULL;
    memblock_alloc_device(ctx, &mem_5654, 8, "mem_5654");
    
    int64_t binop_x_5650 = sext_i32_i64(max_num_groups_5139);
    int64_t bytes_5649 = 8 * binop_x_5650;
    struct memblock_local mem_5651;
    
    mem_5651.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5183, 0, bytes_5649,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5183, 1,
                                  sizeof(num_groups_5144), &num_groups_5144));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5183, 2,
                                  sizeof(mem_5648.mem), &mem_5648.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5183, 3,
                                  sizeof(mem_5654.mem), &mem_5654.mem));
    if (1 * max_num_groups_5139 != 0) {
        const size_t global_work_sizze_5914[1] = {max_num_groups_5139};
        const size_t local_work_sizze_5918[1] = {max_num_groups_5139};
        int64_t time_start_5915 = 0, time_end_5916 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5183");
            fprintf(stderr, "%zu", global_work_sizze_5914[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5918[0]);
            fprintf(stderr, "].\n");
            time_start_5915 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5183, 1, NULL,
                                              global_work_sizze_5914,
                                              local_work_sizze_5918, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5916 = get_wall_time();
            
            long time_diff_5917 = time_end_5916 - time_start_5915;
            
            ctx->reduce_kernel_5183_total_runtime += time_diff_5917;
            ctx->reduce_kernel_5183_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5183",
                    time_diff_5917);
        }
    }
    memblock_unref_device(ctx, &mem_5648, "mem_5648");
    memblock_unref_local(ctx, &mem_5651, "mem_5651");
    
    double read_res_5919;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5654.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5919, 0,
                                       NULL, NULL));
    
    double res_4866 = read_res_5919;
    
    memblock_unref_device(ctx, &mem_5654, "mem_5654");
    
    double y_4873 = res_4859 - 1.0;
    double res_4874 = res_4866 / y_4873;
    
    scalar_out_5723 = res_4874;
    *out_scalar_out_5897 = scalar_out_5723;
    memblock_unref_local(ctx, &mem_5651, "mem_5651");
    memblock_unref_device(ctx, &mem_5654, "mem_5654");
    memblock_unref_local(ctx, &mem_5645, "mem_5645");
    memblock_unref_device(ctx, &mem_5648, "mem_5648");
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    return 0;
}
static int futrts_skew(struct futhark_gpu_context *ctx, double *out_scalar_out_5920,
                       int64_t values_mem_sizze_5629,
                       struct memblock_device values_mem_5630,
                       int32_t sizze_4875)
{
    double scalar_out_5758;
    double res_4877 = sitofp_i32_f64(sizze_4875);
    int32_t group_sizze_5200;
    
    group_sizze_5200 = ctx->sizes.group_sizze_5199;
    
    int32_t max_num_groups_5202;
    
    max_num_groups_5202 = ctx->sizes.max_num_groups_5201;
    
    int32_t y_5203 = group_sizze_5200 - 1;
    int32_t x_5204 = sizze_4875 + y_5203;
    int32_t w_div_group_sizze_5205 = squot32(x_5204, group_sizze_5200);
    int32_t num_groups_maybe_zzero_5206 = smin32(max_num_groups_5202,
                                                 w_div_group_sizze_5205);
    int32_t num_groups_5207 = smax32(1, num_groups_maybe_zzero_5206);
    int32_t num_threads_5208 = group_sizze_5200 * num_groups_5207;
    int32_t y_5209 = num_threads_5208 - 1;
    int32_t x_5210 = sizze_4875 + y_5209;
    int32_t per_thread_elements_5211 = squot32(x_5210, num_threads_5208);
    int64_t binop_x_5635 = sext_i32_i64(num_groups_5207);
    int64_t bytes_5634 = 8 * binop_x_5635;
    struct memblock_device mem_5636;
    
    mem_5636.references = NULL;
    memblock_alloc_device(ctx, &mem_5636, bytes_5634, "mem_5636");
    
    int64_t binop_x_5632 = sext_i32_i64(group_sizze_5200);
    int64_t bytes_5631 = 8 * binop_x_5632;
    struct memblock_local mem_5633;
    
    mem_5633.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4875);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5216, 0,
                                  bytes_5631, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5216, 1,
                                  sizeof(sizze_4875), &sizze_4875));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5216, 2,
                                  sizeof(num_threads_5208), &num_threads_5208));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5216, 3,
                                  sizeof(per_thread_elements_5211),
                                  &per_thread_elements_5211));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5216, 4,
                                  sizeof(values_mem_5630.mem),
                                  &values_mem_5630.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5216, 5,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    if (1 * (num_groups_5207 * group_sizze_5200) != 0) {
        const size_t global_work_sizze_5921[1] = {num_groups_5207 *
                     group_sizze_5200};
        const size_t local_work_sizze_5925[1] = {group_sizze_5200};
        int64_t time_start_5922 = 0, time_end_5923 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5216");
            fprintf(stderr, "%zu", global_work_sizze_5921[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5925[0]);
            fprintf(stderr, "].\n");
            time_start_5922 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5216,
                                              1, NULL, global_work_sizze_5921,
                                              local_work_sizze_5925, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5923 = get_wall_time();
            
            long time_diff_5924 = time_end_5923 - time_start_5922;
            
            ctx->chunked_reduce_kernel_5216_total_runtime += time_diff_5924;
            ctx->chunked_reduce_kernel_5216_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5216", time_diff_5924);
        }
    }
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    
    struct memblock_device mem_5642;
    
    mem_5642.references = NULL;
    memblock_alloc_device(ctx, &mem_5642, 8, "mem_5642");
    
    int64_t binop_x_5638 = sext_i32_i64(max_num_groups_5202);
    int64_t bytes_5637 = 8 * binop_x_5638;
    struct memblock_local mem_5639;
    
    mem_5639.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5244, 0, bytes_5637,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5244, 1,
                                  sizeof(num_groups_5207), &num_groups_5207));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5244, 2,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5244, 3,
                                  sizeof(mem_5642.mem), &mem_5642.mem));
    if (1 * max_num_groups_5202 != 0) {
        const size_t global_work_sizze_5926[1] = {max_num_groups_5202};
        const size_t local_work_sizze_5930[1] = {max_num_groups_5202};
        int64_t time_start_5927 = 0, time_end_5928 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5244");
            fprintf(stderr, "%zu", global_work_sizze_5926[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5930[0]);
            fprintf(stderr, "].\n");
            time_start_5927 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5244, 1, NULL,
                                              global_work_sizze_5926,
                                              local_work_sizze_5930, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5928 = get_wall_time();
            
            long time_diff_5929 = time_end_5928 - time_start_5927;
            
            ctx->reduce_kernel_5244_total_runtime += time_diff_5929;
            ctx->reduce_kernel_5244_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5244",
                    time_diff_5929);
        }
    }
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    
    double read_res_5931;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5642.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5931, 0,
                                       NULL, NULL));
    
    double res_4878 = read_res_5931;
    
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    
    double res_4883 = res_4878 / res_4877;
    int32_t group_sizze_5263;
    
    group_sizze_5263 = ctx->sizes.group_sizze_5262;
    
    int32_t max_num_groups_5265;
    
    max_num_groups_5265 = ctx->sizes.max_num_groups_5264;
    
    int32_t y_5266 = group_sizze_5263 - 1;
    int32_t x_5267 = sizze_4875 + y_5266;
    int32_t w_div_group_sizze_5268 = squot32(x_5267, group_sizze_5263);
    int32_t num_groups_maybe_zzero_5269 = smin32(max_num_groups_5265,
                                                 w_div_group_sizze_5268);
    int32_t num_groups_5270 = smax32(1, num_groups_maybe_zzero_5269);
    int32_t num_threads_5271 = group_sizze_5263 * num_groups_5270;
    int32_t y_5272 = num_threads_5271 - 1;
    int32_t x_5273 = sizze_4875 + y_5272;
    int32_t per_thread_elements_5274 = squot32(x_5273, num_threads_5271);
    int64_t binop_x_5650 = sext_i32_i64(num_groups_5270);
    int64_t bytes_5649 = 8 * binop_x_5650;
    struct memblock_device mem_5651;
    
    mem_5651.references = NULL;
    memblock_alloc_device(ctx, &mem_5651, bytes_5649, "mem_5651");
    
    struct memblock_device mem_5654;
    
    mem_5654.references = NULL;
    memblock_alloc_device(ctx, &mem_5654, bytes_5649, "mem_5654");
    
    int64_t binop_x_5644 = sext_i32_i64(group_sizze_5263);
    int64_t bytes_5643 = 8 * binop_x_5644;
    struct memblock_local mem_5645;
    
    mem_5645.references = NULL;
    
    struct memblock_local mem_5648;
    
    mem_5648.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4875);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5280, 0,
                                  bytes_5643, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5280, 1,
                                  bytes_5643, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5280, 2,
                                  sizeof(sizze_4875), &sizze_4875));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5280, 3,
                                  sizeof(res_4883), &res_4883));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5280, 4,
                                  sizeof(num_threads_5271), &num_threads_5271));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5280, 5,
                                  sizeof(per_thread_elements_5274),
                                  &per_thread_elements_5274));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5280, 6,
                                  sizeof(values_mem_5630.mem),
                                  &values_mem_5630.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5280, 7,
                                  sizeof(mem_5651.mem), &mem_5651.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5280, 8,
                                  sizeof(mem_5654.mem), &mem_5654.mem));
    if (1 * (num_groups_5270 * group_sizze_5263) != 0) {
        const size_t global_work_sizze_5932[1] = {num_groups_5270 *
                     group_sizze_5263};
        const size_t local_work_sizze_5936[1] = {group_sizze_5263};
        int64_t time_start_5933 = 0, time_end_5934 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5280");
            fprintf(stderr, "%zu", global_work_sizze_5932[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5936[0]);
            fprintf(stderr, "].\n");
            time_start_5933 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5280,
                                              1, NULL, global_work_sizze_5932,
                                              local_work_sizze_5936, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5934 = get_wall_time();
            
            long time_diff_5935 = time_end_5934 - time_start_5933;
            
            ctx->chunked_reduce_kernel_5280_total_runtime += time_diff_5935;
            ctx->chunked_reduce_kernel_5280_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5280", time_diff_5935);
        }
    }
    memblock_unref_local(ctx, &mem_5645, "mem_5645");
    memblock_unref_local(ctx, &mem_5648, "mem_5648");
    
    struct memblock_device mem_5663;
    
    mem_5663.references = NULL;
    memblock_alloc_device(ctx, &mem_5663, 8, "mem_5663");
    
    struct memblock_device mem_5666;
    
    mem_5666.references = NULL;
    memblock_alloc_device(ctx, &mem_5666, 8, "mem_5666");
    
    int64_t binop_x_5656 = sext_i32_i64(max_num_groups_5265);
    int64_t bytes_5655 = 8 * binop_x_5656;
    struct memblock_local mem_5657;
    
    mem_5657.references = NULL;
    
    struct memblock_local mem_5660;
    
    mem_5660.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5328, 0, bytes_5655,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5328, 1, bytes_5655,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5328, 2,
                                  sizeof(num_groups_5270), &num_groups_5270));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5328, 3,
                                  sizeof(mem_5651.mem), &mem_5651.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5328, 4,
                                  sizeof(mem_5654.mem), &mem_5654.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5328, 5,
                                  sizeof(mem_5663.mem), &mem_5663.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5328, 6,
                                  sizeof(mem_5666.mem), &mem_5666.mem));
    if (1 * max_num_groups_5265 != 0) {
        const size_t global_work_sizze_5937[1] = {max_num_groups_5265};
        const size_t local_work_sizze_5941[1] = {max_num_groups_5265};
        int64_t time_start_5938 = 0, time_end_5939 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5328");
            fprintf(stderr, "%zu", global_work_sizze_5937[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5941[0]);
            fprintf(stderr, "].\n");
            time_start_5938 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5328, 1, NULL,
                                              global_work_sizze_5937,
                                              local_work_sizze_5941, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5939 = get_wall_time();
            
            long time_diff_5940 = time_end_5939 - time_start_5938;
            
            ctx->reduce_kernel_5328_total_runtime += time_diff_5940;
            ctx->reduce_kernel_5328_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5328",
                    time_diff_5940);
        }
    }
    memblock_unref_device(ctx, &mem_5651, "mem_5651");
    memblock_unref_device(ctx, &mem_5654, "mem_5654");
    memblock_unref_local(ctx, &mem_5657, "mem_5657");
    memblock_unref_local(ctx, &mem_5660, "mem_5660");
    
    double read_res_5942;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5663.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5942, 0,
                                       NULL, NULL));
    
    double res_4884 = read_res_5942;
    
    memblock_unref_device(ctx, &mem_5663, "mem_5663");
    
    double read_res_5943;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5666.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5943, 0,
                                       NULL, NULL));
    
    double res_4885 = read_res_5943;
    
    memblock_unref_device(ctx, &mem_5666, "mem_5666");
    
    double res_4896;
    
    res_4896 = futrts_sqrt64(res_4884);
    
    double res_4897;
    
    res_4897 = futrts_sqrt64(res_4877);
    
    double x_4898 = res_4885 * res_4897;
    double x_4899 = res_4896 * res_4896;
    double y_4900 = res_4896 * x_4899;
    double res_4901 = x_4898 / y_4900;
    
    scalar_out_5758 = res_4901;
    *out_scalar_out_5920 = scalar_out_5758;
    memblock_unref_local(ctx, &mem_5660, "mem_5660");
    memblock_unref_local(ctx, &mem_5657, "mem_5657");
    memblock_unref_device(ctx, &mem_5666, "mem_5666");
    memblock_unref_device(ctx, &mem_5663, "mem_5663");
    memblock_unref_local(ctx, &mem_5648, "mem_5648");
    memblock_unref_local(ctx, &mem_5645, "mem_5645");
    memblock_unref_device(ctx, &mem_5654, "mem_5654");
    memblock_unref_device(ctx, &mem_5651, "mem_5651");
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    return 0;
}
static int futrts_kurtosis(struct futhark_gpu_context *ctx,
                           double *out_scalar_out_5944,
                           int64_t values_mem_sizze_5629,
                           struct memblock_device values_mem_5630,
                           int32_t sizze_4902)
{
    double scalar_out_5798;
    double res_4904 = sitofp_i32_f64(sizze_4902);
    int32_t group_sizze_5349;
    
    group_sizze_5349 = ctx->sizes.group_sizze_5348;
    
    int32_t max_num_groups_5351;
    
    max_num_groups_5351 = ctx->sizes.max_num_groups_5350;
    
    int32_t y_5352 = group_sizze_5349 - 1;
    int32_t x_5353 = sizze_4902 + y_5352;
    int32_t w_div_group_sizze_5354 = squot32(x_5353, group_sizze_5349);
    int32_t num_groups_maybe_zzero_5355 = smin32(max_num_groups_5351,
                                                 w_div_group_sizze_5354);
    int32_t num_groups_5356 = smax32(1, num_groups_maybe_zzero_5355);
    int32_t num_threads_5357 = group_sizze_5349 * num_groups_5356;
    int32_t y_5358 = num_threads_5357 - 1;
    int32_t x_5359 = sizze_4902 + y_5358;
    int32_t per_thread_elements_5360 = squot32(x_5359, num_threads_5357);
    int64_t binop_x_5635 = sext_i32_i64(num_groups_5356);
    int64_t bytes_5634 = 8 * binop_x_5635;
    struct memblock_device mem_5636;
    
    mem_5636.references = NULL;
    memblock_alloc_device(ctx, &mem_5636, bytes_5634, "mem_5636");
    
    int64_t binop_x_5632 = sext_i32_i64(group_sizze_5349);
    int64_t bytes_5631 = 8 * binop_x_5632;
    struct memblock_local mem_5633;
    
    mem_5633.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4902);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5365, 0,
                                  bytes_5631, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5365, 1,
                                  sizeof(sizze_4902), &sizze_4902));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5365, 2,
                                  sizeof(num_threads_5357), &num_threads_5357));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5365, 3,
                                  sizeof(per_thread_elements_5360),
                                  &per_thread_elements_5360));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5365, 4,
                                  sizeof(values_mem_5630.mem),
                                  &values_mem_5630.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5365, 5,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    if (1 * (num_groups_5356 * group_sizze_5349) != 0) {
        const size_t global_work_sizze_5945[1] = {num_groups_5356 *
                     group_sizze_5349};
        const size_t local_work_sizze_5949[1] = {group_sizze_5349};
        int64_t time_start_5946 = 0, time_end_5947 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5365");
            fprintf(stderr, "%zu", global_work_sizze_5945[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5949[0]);
            fprintf(stderr, "].\n");
            time_start_5946 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5365,
                                              1, NULL, global_work_sizze_5945,
                                              local_work_sizze_5949, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5947 = get_wall_time();
            
            long time_diff_5948 = time_end_5947 - time_start_5946;
            
            ctx->chunked_reduce_kernel_5365_total_runtime += time_diff_5948;
            ctx->chunked_reduce_kernel_5365_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5365", time_diff_5948);
        }
    }
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    
    struct memblock_device mem_5642;
    
    mem_5642.references = NULL;
    memblock_alloc_device(ctx, &mem_5642, 8, "mem_5642");
    
    int64_t binop_x_5638 = sext_i32_i64(max_num_groups_5351);
    int64_t bytes_5637 = 8 * binop_x_5638;
    struct memblock_local mem_5639;
    
    mem_5639.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5393, 0, bytes_5637,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5393, 1,
                                  sizeof(num_groups_5356), &num_groups_5356));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5393, 2,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5393, 3,
                                  sizeof(mem_5642.mem), &mem_5642.mem));
    if (1 * max_num_groups_5351 != 0) {
        const size_t global_work_sizze_5950[1] = {max_num_groups_5351};
        const size_t local_work_sizze_5954[1] = {max_num_groups_5351};
        int64_t time_start_5951 = 0, time_end_5952 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5393");
            fprintf(stderr, "%zu", global_work_sizze_5950[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5954[0]);
            fprintf(stderr, "].\n");
            time_start_5951 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5393, 1, NULL,
                                              global_work_sizze_5950,
                                              local_work_sizze_5954, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5952 = get_wall_time();
            
            long time_diff_5953 = time_end_5952 - time_start_5951;
            
            ctx->reduce_kernel_5393_total_runtime += time_diff_5953;
            ctx->reduce_kernel_5393_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5393",
                    time_diff_5953);
        }
    }
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    
    double read_res_5955;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5642.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5955, 0,
                                       NULL, NULL));
    
    double res_4905 = read_res_5955;
    
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    
    double res_4910 = res_4905 / res_4904;
    int32_t group_sizze_5412;
    
    group_sizze_5412 = ctx->sizes.group_sizze_5411;
    
    int32_t max_num_groups_5414;
    
    max_num_groups_5414 = ctx->sizes.max_num_groups_5413;
    
    int32_t y_5415 = group_sizze_5412 - 1;
    int32_t x_5416 = sizze_4902 + y_5415;
    int32_t w_div_group_sizze_5417 = squot32(x_5416, group_sizze_5412);
    int32_t num_groups_maybe_zzero_5418 = smin32(max_num_groups_5414,
                                                 w_div_group_sizze_5417);
    int32_t num_groups_5419 = smax32(1, num_groups_maybe_zzero_5418);
    int32_t num_threads_5420 = group_sizze_5412 * num_groups_5419;
    int32_t y_5421 = num_threads_5420 - 1;
    int32_t x_5422 = sizze_4902 + y_5421;
    int32_t per_thread_elements_5423 = squot32(x_5422, num_threads_5420);
    int64_t binop_x_5650 = sext_i32_i64(num_groups_5419);
    int64_t bytes_5649 = 8 * binop_x_5650;
    struct memblock_device mem_5651;
    
    mem_5651.references = NULL;
    memblock_alloc_device(ctx, &mem_5651, bytes_5649, "mem_5651");
    
    struct memblock_device mem_5654;
    
    mem_5654.references = NULL;
    memblock_alloc_device(ctx, &mem_5654, bytes_5649, "mem_5654");
    
    int64_t binop_x_5644 = sext_i32_i64(group_sizze_5412);
    int64_t bytes_5643 = 8 * binop_x_5644;
    struct memblock_local mem_5645;
    
    mem_5645.references = NULL;
    
    struct memblock_local mem_5648;
    
    mem_5648.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4902);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5429, 0,
                                  bytes_5643, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5429, 1,
                                  bytes_5643, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5429, 2,
                                  sizeof(sizze_4902), &sizze_4902));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5429, 3,
                                  sizeof(res_4910), &res_4910));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5429, 4,
                                  sizeof(num_threads_5420), &num_threads_5420));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5429, 5,
                                  sizeof(per_thread_elements_5423),
                                  &per_thread_elements_5423));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5429, 6,
                                  sizeof(values_mem_5630.mem),
                                  &values_mem_5630.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5429, 7,
                                  sizeof(mem_5651.mem), &mem_5651.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5429, 8,
                                  sizeof(mem_5654.mem), &mem_5654.mem));
    if (1 * (num_groups_5419 * group_sizze_5412) != 0) {
        const size_t global_work_sizze_5956[1] = {num_groups_5419 *
                     group_sizze_5412};
        const size_t local_work_sizze_5960[1] = {group_sizze_5412};
        int64_t time_start_5957 = 0, time_end_5958 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5429");
            fprintf(stderr, "%zu", global_work_sizze_5956[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5960[0]);
            fprintf(stderr, "].\n");
            time_start_5957 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5429,
                                              1, NULL, global_work_sizze_5956,
                                              local_work_sizze_5960, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5958 = get_wall_time();
            
            long time_diff_5959 = time_end_5958 - time_start_5957;
            
            ctx->chunked_reduce_kernel_5429_total_runtime += time_diff_5959;
            ctx->chunked_reduce_kernel_5429_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5429", time_diff_5959);
        }
    }
    memblock_unref_local(ctx, &mem_5645, "mem_5645");
    memblock_unref_local(ctx, &mem_5648, "mem_5648");
    
    struct memblock_device mem_5663;
    
    mem_5663.references = NULL;
    memblock_alloc_device(ctx, &mem_5663, 8, "mem_5663");
    
    struct memblock_device mem_5666;
    
    mem_5666.references = NULL;
    memblock_alloc_device(ctx, &mem_5666, 8, "mem_5666");
    
    int64_t binop_x_5656 = sext_i32_i64(max_num_groups_5414);
    int64_t bytes_5655 = 8 * binop_x_5656;
    struct memblock_local mem_5657;
    
    mem_5657.references = NULL;
    
    struct memblock_local mem_5660;
    
    mem_5660.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5478, 0, bytes_5655,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5478, 1, bytes_5655,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5478, 2,
                                  sizeof(num_groups_5419), &num_groups_5419));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5478, 3,
                                  sizeof(mem_5651.mem), &mem_5651.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5478, 4,
                                  sizeof(mem_5654.mem), &mem_5654.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5478, 5,
                                  sizeof(mem_5663.mem), &mem_5663.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5478, 6,
                                  sizeof(mem_5666.mem), &mem_5666.mem));
    if (1 * max_num_groups_5414 != 0) {
        const size_t global_work_sizze_5961[1] = {max_num_groups_5414};
        const size_t local_work_sizze_5965[1] = {max_num_groups_5414};
        int64_t time_start_5962 = 0, time_end_5963 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5478");
            fprintf(stderr, "%zu", global_work_sizze_5961[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5965[0]);
            fprintf(stderr, "].\n");
            time_start_5962 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5478, 1, NULL,
                                              global_work_sizze_5961,
                                              local_work_sizze_5965, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5963 = get_wall_time();
            
            long time_diff_5964 = time_end_5963 - time_start_5962;
            
            ctx->reduce_kernel_5478_total_runtime += time_diff_5964;
            ctx->reduce_kernel_5478_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5478",
                    time_diff_5964);
        }
    }
    memblock_unref_device(ctx, &mem_5651, "mem_5651");
    memblock_unref_device(ctx, &mem_5654, "mem_5654");
    memblock_unref_local(ctx, &mem_5657, "mem_5657");
    memblock_unref_local(ctx, &mem_5660, "mem_5660");
    
    double read_res_5966;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5663.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5966, 0,
                                       NULL, NULL));
    
    double res_4911 = read_res_5966;
    
    memblock_unref_device(ctx, &mem_5663, "mem_5663");
    
    double read_res_5967;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5666.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5967, 0,
                                       NULL, NULL));
    
    double res_4912 = read_res_5967;
    
    memblock_unref_device(ctx, &mem_5666, "mem_5666");
    
    double x_4924 = res_4904 * res_4912;
    double y_4925 = res_4911 * res_4911;
    double res_4926 = x_4924 / y_4925;
    
    scalar_out_5798 = res_4926;
    *out_scalar_out_5944 = scalar_out_5798;
    memblock_unref_local(ctx, &mem_5660, "mem_5660");
    memblock_unref_local(ctx, &mem_5657, "mem_5657");
    memblock_unref_device(ctx, &mem_5666, "mem_5666");
    memblock_unref_device(ctx, &mem_5663, "mem_5663");
    memblock_unref_local(ctx, &mem_5648, "mem_5648");
    memblock_unref_local(ctx, &mem_5645, "mem_5645");
    memblock_unref_device(ctx, &mem_5654, "mem_5654");
    memblock_unref_device(ctx, &mem_5651, "mem_5651");
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    return 0;
}
static int futrts_stddev(struct futhark_gpu_context *ctx,
                         double *out_scalar_out_5968,
                         int64_t values_mem_sizze_5629,
                         struct memblock_device values_mem_5630,
                         int32_t sizze_4927)
{
    double scalar_out_5838;
    double res_4929 = sitofp_i32_f64(sizze_4927);
    int32_t group_sizze_5499;
    
    group_sizze_5499 = ctx->sizes.group_sizze_5498;
    
    int32_t max_num_groups_5501;
    
    max_num_groups_5501 = ctx->sizes.max_num_groups_5500;
    
    int32_t y_5502 = group_sizze_5499 - 1;
    int32_t x_5503 = sizze_4927 + y_5502;
    int32_t w_div_group_sizze_5504 = squot32(x_5503, group_sizze_5499);
    int32_t num_groups_maybe_zzero_5505 = smin32(max_num_groups_5501,
                                                 w_div_group_sizze_5504);
    int32_t num_groups_5506 = smax32(1, num_groups_maybe_zzero_5505);
    int32_t num_threads_5507 = group_sizze_5499 * num_groups_5506;
    int32_t y_5508 = num_threads_5507 - 1;
    int32_t x_5509 = sizze_4927 + y_5508;
    int32_t per_thread_elements_5510 = squot32(x_5509, num_threads_5507);
    int64_t binop_x_5635 = sext_i32_i64(num_groups_5506);
    int64_t bytes_5634 = 8 * binop_x_5635;
    struct memblock_device mem_5636;
    
    mem_5636.references = NULL;
    memblock_alloc_device(ctx, &mem_5636, bytes_5634, "mem_5636");
    
    int64_t binop_x_5632 = sext_i32_i64(group_sizze_5499);
    int64_t bytes_5631 = 8 * binop_x_5632;
    struct memblock_local mem_5633;
    
    mem_5633.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4927);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5515, 0,
                                  bytes_5631, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5515, 1,
                                  sizeof(sizze_4927), &sizze_4927));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5515, 2,
                                  sizeof(num_threads_5507), &num_threads_5507));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5515, 3,
                                  sizeof(per_thread_elements_5510),
                                  &per_thread_elements_5510));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5515, 4,
                                  sizeof(values_mem_5630.mem),
                                  &values_mem_5630.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5515, 5,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    if (1 * (num_groups_5506 * group_sizze_5499) != 0) {
        const size_t global_work_sizze_5969[1] = {num_groups_5506 *
                     group_sizze_5499};
        const size_t local_work_sizze_5973[1] = {group_sizze_5499};
        int64_t time_start_5970 = 0, time_end_5971 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5515");
            fprintf(stderr, "%zu", global_work_sizze_5969[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5973[0]);
            fprintf(stderr, "].\n");
            time_start_5970 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5515,
                                              1, NULL, global_work_sizze_5969,
                                              local_work_sizze_5973, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5971 = get_wall_time();
            
            long time_diff_5972 = time_end_5971 - time_start_5970;
            
            ctx->chunked_reduce_kernel_5515_total_runtime += time_diff_5972;
            ctx->chunked_reduce_kernel_5515_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5515", time_diff_5972);
        }
    }
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    
    struct memblock_device mem_5642;
    
    mem_5642.references = NULL;
    memblock_alloc_device(ctx, &mem_5642, 8, "mem_5642");
    
    int64_t binop_x_5638 = sext_i32_i64(max_num_groups_5501);
    int64_t bytes_5637 = 8 * binop_x_5638;
    struct memblock_local mem_5639;
    
    mem_5639.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5543, 0, bytes_5637,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5543, 1,
                                  sizeof(num_groups_5506), &num_groups_5506));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5543, 2,
                                  sizeof(mem_5636.mem), &mem_5636.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5543, 3,
                                  sizeof(mem_5642.mem), &mem_5642.mem));
    if (1 * max_num_groups_5501 != 0) {
        const size_t global_work_sizze_5974[1] = {max_num_groups_5501};
        const size_t local_work_sizze_5978[1] = {max_num_groups_5501};
        int64_t time_start_5975 = 0, time_end_5976 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5543");
            fprintf(stderr, "%zu", global_work_sizze_5974[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5978[0]);
            fprintf(stderr, "].\n");
            time_start_5975 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5543, 1, NULL,
                                              global_work_sizze_5974,
                                              local_work_sizze_5978, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5976 = get_wall_time();
            
            long time_diff_5977 = time_end_5976 - time_start_5975;
            
            ctx->reduce_kernel_5543_total_runtime += time_diff_5977;
            ctx->reduce_kernel_5543_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5543",
                    time_diff_5977);
        }
    }
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    
    double read_res_5979;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5642.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5979, 0,
                                       NULL, NULL));
    
    double res_4930 = read_res_5979;
    
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    
    double res_4935 = res_4930 / res_4929;
    int32_t group_sizze_5560;
    
    group_sizze_5560 = ctx->sizes.group_sizze_5559;
    
    int32_t max_num_groups_5562;
    
    max_num_groups_5562 = ctx->sizes.max_num_groups_5561;
    
    int32_t y_5563 = group_sizze_5560 - 1;
    int32_t x_5564 = sizze_4927 + y_5563;
    int32_t w_div_group_sizze_5565 = squot32(x_5564, group_sizze_5560);
    int32_t num_groups_maybe_zzero_5566 = smin32(max_num_groups_5562,
                                                 w_div_group_sizze_5565);
    int32_t num_groups_5567 = smax32(1, num_groups_maybe_zzero_5566);
    int32_t num_threads_5568 = group_sizze_5560 * num_groups_5567;
    int32_t y_5569 = num_threads_5568 - 1;
    int32_t x_5570 = sizze_4927 + y_5569;
    int32_t per_thread_elements_5571 = squot32(x_5570, num_threads_5568);
    int64_t binop_x_5647 = sext_i32_i64(num_groups_5567);
    int64_t bytes_5646 = 8 * binop_x_5647;
    struct memblock_device mem_5648;
    
    mem_5648.references = NULL;
    memblock_alloc_device(ctx, &mem_5648, bytes_5646, "mem_5648");
    
    int64_t binop_x_5644 = sext_i32_i64(group_sizze_5560);
    int64_t bytes_5643 = 8 * binop_x_5644;
    struct memblock_local mem_5645;
    
    mem_5645.references = NULL;
    if (ctx->debugging)
        fprintf(stderr, "%s: %d\n", "input size", (int) sizze_4927);
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5576, 0,
                                  bytes_5643, NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5576, 1,
                                  sizeof(sizze_4927), &sizze_4927));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5576, 2,
                                  sizeof(res_4935), &res_4935));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5576, 3,
                                  sizeof(num_threads_5568), &num_threads_5568));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5576, 4,
                                  sizeof(per_thread_elements_5571),
                                  &per_thread_elements_5571));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5576, 5,
                                  sizeof(values_mem_5630.mem),
                                  &values_mem_5630.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->chunked_reduce_kernel_5576, 6,
                                  sizeof(mem_5648.mem), &mem_5648.mem));
    if (1 * (num_groups_5567 * group_sizze_5560) != 0) {
        const size_t global_work_sizze_5980[1] = {num_groups_5567 *
                     group_sizze_5560};
        const size_t local_work_sizze_5984[1] = {group_sizze_5560};
        int64_t time_start_5981 = 0, time_end_5982 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "chunked_reduce_kernel_5576");
            fprintf(stderr, "%zu", global_work_sizze_5980[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5984[0]);
            fprintf(stderr, "].\n");
            time_start_5981 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->chunked_reduce_kernel_5576,
                                              1, NULL, global_work_sizze_5980,
                                              local_work_sizze_5984, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5982 = get_wall_time();
            
            long time_diff_5983 = time_end_5982 - time_start_5981;
            
            ctx->chunked_reduce_kernel_5576_total_runtime += time_diff_5983;
            ctx->chunked_reduce_kernel_5576_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n",
                    "chunked_reduce_kernel_5576", time_diff_5983);
        }
    }
    memblock_unref_local(ctx, &mem_5645, "mem_5645");
    
    struct memblock_device mem_5654;
    
    mem_5654.references = NULL;
    memblock_alloc_device(ctx, &mem_5654, 8, "mem_5654");
    
    int64_t binop_x_5650 = sext_i32_i64(max_num_groups_5562);
    int64_t bytes_5649 = 8 * binop_x_5650;
    struct memblock_local mem_5651;
    
    mem_5651.references = NULL;
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5606, 0, bytes_5649,
                                  NULL));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5606, 1,
                                  sizeof(num_groups_5567), &num_groups_5567));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5606, 2,
                                  sizeof(mem_5648.mem), &mem_5648.mem));
    OPENCL_SUCCEED(clSetKernelArg(ctx->reduce_kernel_5606, 3,
                                  sizeof(mem_5654.mem), &mem_5654.mem));
    if (1 * max_num_groups_5562 != 0) {
        const size_t global_work_sizze_5985[1] = {max_num_groups_5562};
        const size_t local_work_sizze_5989[1] = {max_num_groups_5562};
        int64_t time_start_5986 = 0, time_end_5987 = 0;
        
        if (ctx->debugging) {
            fprintf(stderr, "Launching %s with global work size [",
                    "reduce_kernel_5606");
            fprintf(stderr, "%zu", global_work_sizze_5985[0]);
            fprintf(stderr, "] and local work size [");
            fprintf(stderr, "%zu", local_work_sizze_5989[0]);
            fprintf(stderr, "].\n");
            time_start_5986 = get_wall_time();
        }
        OPENCL_SUCCEED(clEnqueueNDRangeKernel(ctx->opencl.queue,
                                              ctx->reduce_kernel_5606, 1, NULL,
                                              global_work_sizze_5985,
                                              local_work_sizze_5989, 0, NULL,
                                              NULL));
        if (ctx->debugging) {
            OPENCL_SUCCEED(clFinish(ctx->opencl.queue));
            time_end_5987 = get_wall_time();
            
            long time_diff_5988 = time_end_5987 - time_start_5986;
            
            ctx->reduce_kernel_5606_total_runtime += time_diff_5988;
            ctx->reduce_kernel_5606_runs++;
            fprintf(stderr, "kernel %s runtime: %ldus\n", "reduce_kernel_5606",
                    time_diff_5988);
        }
    }
    memblock_unref_device(ctx, &mem_5648, "mem_5648");
    memblock_unref_local(ctx, &mem_5651, "mem_5651");
    
    double read_res_5990;
    
    OPENCL_SUCCEED(clEnqueueReadBuffer(ctx->opencl.queue, mem_5654.mem, CL_TRUE,
                                       0, sizeof(double), &read_res_5990, 0,
                                       NULL, NULL));
    
    double res_4936 = read_res_5990;
    
    memblock_unref_device(ctx, &mem_5654, "mem_5654");
    
    double y_4943 = res_4929 - 1.0;
    double res_4944 = res_4936 / y_4943;
    double res_4945;
    
    res_4945 = futrts_sqrt64(res_4944);
    scalar_out_5838 = res_4945;
    *out_scalar_out_5968 = scalar_out_5838;
    memblock_unref_local(ctx, &mem_5651, "mem_5651");
    memblock_unref_device(ctx, &mem_5654, "mem_5654");
    memblock_unref_local(ctx, &mem_5645, "mem_5645");
    memblock_unref_device(ctx, &mem_5648, "mem_5648");
    memblock_unref_local(ctx, &mem_5639, "mem_5639");
    memblock_unref_device(ctx, &mem_5642, "mem_5642");
    memblock_unref_local(ctx, &mem_5633, "mem_5633");
    memblock_unref_device(ctx, &mem_5636, "mem_5636");
    return 0;
}
struct futhark_gpu_f64_1d {
    struct memblock_device mem;
    int64_t shape[1];
} ;
struct futhark_gpu_f64_1d *futhark_gpu_new_f64_1d(struct futhark_gpu_context *ctx,
                                          double *data, int dim0)
{
    struct futhark_gpu_f64_1d *arr = malloc(sizeof(struct futhark_gpu_f64_1d));
    
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
struct futhark_gpu_f64_1d *futhark_gpu_new_raw_f64_1d(struct futhark_gpu_context *ctx,
                                              cl_mem data, int offset, int dim0)
{
    struct futhark_gpu_f64_1d *arr = malloc(sizeof(struct futhark_gpu_f64_1d));
    
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
int futhark_gpu_free_f64_1d(struct futhark_gpu_context *ctx, struct futhark_gpu_f64_1d *arr)
{
    lock_lock(&ctx->lock);
    memblock_unref_device(ctx, &arr->mem, "arr->mem");
    lock_unlock(&ctx->lock);
    free(arr);
    return 0;
}
int futhark_gpu_values_f64_1d(struct futhark_gpu_context *ctx,
                          struct futhark_gpu_f64_1d *arr, double *data)
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
cl_mem futhark_gpu_values_raw_f64_1d(struct futhark_gpu_context *ctx,
                                 struct futhark_gpu_f64_1d *arr)
{
    return arr->mem.mem;
}
int64_t *futhark_gpu_shape_f64_1d(struct futhark_gpu_context *ctx,
                              struct futhark_gpu_f64_1d *arr)
{
    return arr->shape;
}
int futhark_gpu_entry_sum(struct futhark_gpu_context *ctx, double *out0, const
                      struct futhark_gpu_f64_1d *in0)
{
    int64_t col_mem_sizze_5629;
    struct memblock_device col_mem_5630;
    
    col_mem_5630.references = NULL;
    
    int32_t sizze_4841;
    double scalar_out_5687;
    
    lock_lock(&ctx->lock);
    col_mem_5630 = in0->mem;
    col_mem_sizze_5629 = in0->mem.size;
    sizze_4841 = in0->shape[0];
    
    int ret = futrts_sum(ctx, &scalar_out_5687, col_mem_sizze_5629,
                         col_mem_5630, sizze_4841);
    
    if (ret == 0) {
        *out0 = scalar_out_5687;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_gpu_entry_mean(struct futhark_gpu_context *ctx, double *out0, const
                       struct futhark_gpu_f64_1d *in0)
{
    int64_t col_mem_sizze_5629;
    struct memblock_device col_mem_5630;
    
    col_mem_5630.references = NULL;
    
    int32_t sizze_4848;
    double scalar_out_5705;
    
    lock_lock(&ctx->lock);
    col_mem_5630 = in0->mem;
    col_mem_sizze_5629 = in0->mem.size;
    sizze_4848 = in0->shape[0];
    
    int ret = futrts_mean(ctx, &scalar_out_5705, col_mem_sizze_5629,
                          col_mem_5630, sizze_4848);
    
    if (ret == 0) {
        *out0 = scalar_out_5705;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_gpu_entry_variance(struct futhark_gpu_context *ctx, double *out0, const
                           struct futhark_gpu_f64_1d *in0)
{
    int64_t values_mem_sizze_5629;
    struct memblock_device values_mem_5630;
    
    values_mem_5630.references = NULL;
    
    int32_t sizze_4857;
    double scalar_out_5723;
    
    lock_lock(&ctx->lock);
    values_mem_5630 = in0->mem;
    values_mem_sizze_5629 = in0->mem.size;
    sizze_4857 = in0->shape[0];
    
    int ret = futrts_variance(ctx, &scalar_out_5723, values_mem_sizze_5629,
                              values_mem_5630, sizze_4857);
    
    if (ret == 0) {
        *out0 = scalar_out_5723;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_gpu_entry_skew(struct futhark_gpu_context *ctx, double *out0, const
                       struct futhark_gpu_f64_1d *in0)
{
    int64_t values_mem_sizze_5629;
    struct memblock_device values_mem_5630;
    
    values_mem_5630.references = NULL;
    
    int32_t sizze_4875;
    double scalar_out_5758;
    
    lock_lock(&ctx->lock);
    values_mem_5630 = in0->mem;
    values_mem_sizze_5629 = in0->mem.size;
    sizze_4875 = in0->shape[0];
    
    int ret = futrts_skew(ctx, &scalar_out_5758, values_mem_sizze_5629,
                          values_mem_5630, sizze_4875);
    
    if (ret == 0) {
        *out0 = scalar_out_5758;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_gpu_entry_kurtosis(struct futhark_gpu_context *ctx, double *out0, const
                           struct futhark_gpu_f64_1d *in0)
{
    int64_t values_mem_sizze_5629;
    struct memblock_device values_mem_5630;
    
    values_mem_5630.references = NULL;
    
    int32_t sizze_4902;
    double scalar_out_5798;
    
    lock_lock(&ctx->lock);
    values_mem_5630 = in0->mem;
    values_mem_sizze_5629 = in0->mem.size;
    sizze_4902 = in0->shape[0];
    
    int ret = futrts_kurtosis(ctx, &scalar_out_5798, values_mem_sizze_5629,
                              values_mem_5630, sizze_4902);
    
    if (ret == 0) {
        *out0 = scalar_out_5798;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
int futhark_gpu_entry_stddev(struct futhark_gpu_context *ctx, double *out0, const
                         struct futhark_gpu_f64_1d *in0)
{
    int64_t values_mem_sizze_5629;
    struct memblock_device values_mem_5630;
    
    values_mem_5630.references = NULL;
    
    int32_t sizze_4927;
    double scalar_out_5838;
    
    lock_lock(&ctx->lock);
    values_mem_5630 = in0->mem;
    values_mem_sizze_5629 = in0->mem.size;
    sizze_4927 = in0->shape[0];
    
    int ret = futrts_stddev(ctx, &scalar_out_5838, values_mem_sizze_5629,
                            values_mem_5630, sizze_4927);
    
    if (ret == 0) {
        *out0 = scalar_out_5838;
    }
    lock_unlock(&ctx->lock);
    return ret;
}
