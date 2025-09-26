#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
blue='\033[0;34m'
yellow='\033[0;33m'
plain='\033[0m'

cur_dir=$(pwd)

REPO_OWNER=${REPO_OWNER:-runetfreedom}
REPO_NAME=${REPO_NAME:-3x-ui}
REPO_REF=${REPO_REF:-master}

# check root
[[ $EUID -ne 0 ]] && echo -e "${red}Fatal error: ${plain} Please run this script with root privilege \n " && exit 1

# Check OS and set release variable
if [[ -f /etc/os-release ]]; then
    source /etc/os-release
    release=$ID
elif [[ -f /usr/lib/os-release ]]; then
    source /usr/lib/os-release
    release=$ID
else
    echo "Failed to check the system OS, please contact the author!" >&2
    exit 1
fi
echo "The OS release is: $release"

arch() {
    case "$(uname -m)" in
    x86_64 | x64 | amd64) echo 'amd64' ;;
    i*86 | x86) echo '386' ;;
    armv8* | armv8 | arm64 | aarch64) echo 'arm64' ;;
    armv7* | armv7 | arm) echo 'armv7' ;;
    armv6* | armv6) echo 'armv6' ;;
    armv5* | armv5) echo 'armv5' ;;
    s390x) echo 's390x' ;;
    *) echo -e "${green}Unsupported CPU architecture! ${plain}" && rm -f install.sh && exit 1 ;;
    esac
}

echo "Arch: $(arch)"

install_base() {
    case "${release}" in
    ubuntu | debian | armbian)
        apt-get update && apt-get install -y -q wget curl tar tzdata unzip golang
        ;;
    centos | rhel | almalinux | rocky | ol)
        yum -y update && yum install -y -q wget curl tar tzdata unzip golang
        ;;
    fedora | amzn | virtuozzo)
        dnf -y update && dnf install -y -q wget curl tar tzdata unzip golang
        ;;
    arch | manjaro | parch)
        pacman -Syu && pacman -Syu --noconfirm wget curl tar tzdata unzip go
        ;;
    opensuse-tumbleweed)
        zypper refresh && zypper -q install -y wget curl tar timezone unzip go
        ;;
    alpine)
        apk update && apk add wget curl tar tzdata unzip go
        ;;
    *)
        apt-get update && apt-get install -y -q wget curl tar tzdata unzip golang
        ;;
    esac
}

ensure_go() {
    if command -v go >/dev/null 2>&1; then
        return 0
    fi

    echo -e "${red}Go binary could not be located. Please install Go manually and re-run the installer.${plain}"
    exit 1
}

gen_random_string() {
    local length="$1"
    local random_string=$(LC_ALL=C tr -dc 'a-zA-Z0-9' </dev/urandom | fold -w "$length" | head -n 1)
    echo "$random_string"
}

config_after_install() {
    local existing_hasDefaultCredential=$(/usr/local/x-ui/x-ui setting -show true | grep -Eo 'hasDefaultCredential: .+' | awk '{print $2}')
    local existing_webBasePath=$(/usr/local/x-ui/x-ui setting -show true | grep -Eo 'webBasePath: .+' | awk '{print $2}')
    local existing_port=$(/usr/local/x-ui/x-ui setting -show true | grep -Eo 'port: .+' | awk '{print $2}')
    local URL_lists=(
        "https://api4.ipify.org"
		"https://ipv4.icanhazip.com"
		"https://v4.api.ipinfo.io/ip"
		"https://ipv4.myexternalip.com/raw"
		"https://4.ident.me"
		"https://check-host.net/ip"
    )
    local server_ip=""
    for ip_address in "${URL_lists[@]}"; do
        server_ip=$(curl -s --max-time 3 "${ip_address}" 2>/dev/null | tr -d '[:space:]')
        if [[ -n "${server_ip}" ]]; then
            break
        fi
    done

    if [[ ${#existing_webBasePath} -lt 4 ]]; then
        if [[ "$existing_hasDefaultCredential" == "true" ]]; then
            local config_webBasePath=$(gen_random_string 18)
            local config_username=$(gen_random_string 10)
            local config_password=$(gen_random_string 10)

            read -rp "Would you like to customize the Panel Port settings? (If not, a random port will be applied) [y/n]: " config_confirm
            if [[ "${config_confirm}" == "y" || "${config_confirm}" == "Y" ]]; then
                read -rp "Please set up the panel port: " config_port
                echo -e "${yellow}Your Panel Port is: ${config_port}${plain}"
            else
                local config_port=$(shuf -i 1024-62000 -n 1)
                echo -e "${yellow}Generated random port: ${config_port}${plain}"
            fi

            /usr/local/x-ui/x-ui setting -username "${config_username}" -password "${config_password}" -port "${config_port}" -webBasePath "${config_webBasePath}"
            echo -e "This is a fresh installation, generating random login info for security concerns:"
            echo -e "###############################################"
            echo -e "${green}Username: ${config_username}${plain}"
            echo -e "${green}Password: ${config_password}${plain}"
            echo -e "${green}Port: ${config_port}${plain}"
            echo -e "${green}WebBasePath: ${config_webBasePath}${plain}"
            echo -e "${green}Access URL: http://${server_ip}:${config_port}/${config_webBasePath}${plain}"
            echo -e "###############################################"
        else
            local config_webBasePath=$(gen_random_string 18)
            echo -e "${yellow}WebBasePath is missing or too short. Generating a new one...${plain}"
            /usr/local/x-ui/x-ui setting -webBasePath "${config_webBasePath}"
            echo -e "${green}New WebBasePath: ${config_webBasePath}${plain}"
            echo -e "${green}Access URL: http://${server_ip}:${existing_port}/${config_webBasePath}${plain}"
        fi
    else
        if [[ "$existing_hasDefaultCredential" == "true" ]]; then
            local config_username=$(gen_random_string 10)
            local config_password=$(gen_random_string 10)

            echo -e "${yellow}Default credentials detected. Security update required...${plain}"
            /usr/local/x-ui/x-ui setting -username "${config_username}" -password "${config_password}"
            echo -e "Generated new random login credentials:"
            echo -e "###############################################"
            echo -e "${green}Username: ${config_username}${plain}"
            echo -e "${green}Password: ${config_password}${plain}"
            echo -e "###############################################"
        else
            echo -e "${green}Username, Password, and WebBasePath are properly set. Exiting...${plain}"
        fi
    fi

    /usr/local/x-ui/x-ui migrate
}

map_xray_asset() {
    local cpu_arch="$1"
    case "${cpu_arch}" in
    amd64)
        echo "64"
        ;;
    386)
        echo "32"
        ;;
    arm64)
        echo "arm64-v8a"
        ;;
    armv7)
        echo "arm32-v7a"
        ;;
    armv6)
        echo "arm32-v6"
        ;;
    armv5)
        echo "arm32-v5"
        ;;
    s390x)
        echo "s390x"
        ;;
    loong64)
        echo "loong64"
        ;;
    mips32)
        echo "mips32"
        ;;
    mips32le)
        echo "mips32le"
        ;;
    mips64)
        echo "mips64"
        ;;
    mips64le)
        echo "mips64le"
        ;;
    ppc64)
        echo "ppc64"
        ;;
    ppc64le)
        echo "ppc64le"
        ;;
    riscv64)
        echo "riscv64"
        ;;
    *)
        return 1
        ;;
    esac
}

install_xray_assets() {
    local target_dir="$1"
    local cpu_arch="$(arch)"
    local asset
    asset=$(map_xray_asset "${cpu_arch}") || {
        echo -e "${red}Unsupported architecture ${cpu_arch} for Xray binary.${plain}"
        exit 1
    }

    local runtime_name="xray-linux-${cpu_arch}"
    if [[ "${cpu_arch}" == "armv7" || "${cpu_arch}" == "armv6" || "${cpu_arch}" == "armv5" ]]; then
        runtime_name="xray-linux-arm"
    fi

    local temp_dir
    temp_dir=$(mktemp -d)
    local xray_url="https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-${asset}.zip"

    echo -e "${green}Downloading Xray-core asset (${asset})...${plain}"
    if ! curl -Lsf "${xray_url}" -o "${temp_dir}/xray.zip"; then
        echo -e "${red}Failed to download Xray asset from ${xray_url}.${plain}"
        rm -rf "${temp_dir}"
        exit 1
    fi

    if ! unzip -q "${temp_dir}/xray.zip" -d "${temp_dir}"; then
        echo -e "${red}Failed to extract Xray archive.${plain}"
        rm -rf "${temp_dir}"
        exit 1
    fi

    install -d "${target_dir}"

    install -m 755 "${temp_dir}/xray" "${target_dir}/${runtime_name}"
    if [[ -f "${temp_dir}/geoip.dat" ]]; then
        install -m 644 "${temp_dir}/geoip.dat" "${target_dir}/geoip.dat"
    fi
    if [[ -f "${temp_dir}/geosite.dat" ]]; then
        install -m 644 "${temp_dir}/geosite.dat" "${target_dir}/geosite.dat"
    fi

    rm -rf "${temp_dir}"
}

install_x-ui() {
    local ref="$REPO_REF"
    if [[ $# -gt 0 ]]; then
        ref="$1"
    fi

    local source_dir
    source_dir=$(mktemp -d)
    local archive_url="https://codeload.github.com/${REPO_OWNER}/${REPO_NAME}/tar.gz/${ref}"

    echo -e "${green}Fetching ${REPO_OWNER}/${REPO_NAME}@${ref} ...${plain}"
    if ! curl -Lsf "${archive_url}" | tar -xz -C "${source_dir}" --strip-components=1; then
        echo -e "${red}Failed to download repository sources from ${archive_url}.${plain}"
        rm -rf "${source_dir}"
        exit 1
    fi

    echo -e "${green}Building backend binary...${plain}"

    local goarch="$(arch)"
    local goos="linux"
    local goarm=""
    case "${goarch}" in
    armv7)
        goarch="arm"
        goarm="7"
        ;;
    armv6)
        goarch="arm"
        goarm="6"
        ;;
    armv5)
        goarch="arm"
        goarm="5"
        ;;
    esac

    pushd "${source_dir}" >/dev/null || exit 1

    if [[ -n "${goarm}" ]]; then
        GOOS="${goos}" GOARCH="${goarch}" GOARM="${goarm}" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o x-ui main.go
    else
        GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o x-ui main.go
    fi

    if [[ $? -ne 0 ]]; then
        echo -e "${red}Go build failed.${plain}"
        popd >/dev/null
        rm -rf "${source_dir}"
        exit 1
    fi

    chmod +x x-ui x-ui.sh

    local install_dir="/usr/local/x-ui"
    if [[ -d "${install_dir}" ]]; then
        if [[ $release == "alpine" ]]; then
            rc-service x-ui stop
        else
            systemctl stop x-ui 2>/dev/null || true
        fi
        rm -rf "${install_dir}"
    fi

    install -d "${install_dir}/bin"

    local runtime_items=(config database logger media sub tor util web windows_files xray)
    for item in "${runtime_items[@]}"; do
        if [[ -e "${item}" ]]; then
            cp -r "${item}" "${install_dir}/"
        fi
    done

    install -m 755 x-ui "${install_dir}/x-ui"
    install -m 755 x-ui.sh "${install_dir}/x-ui.sh"
    if [[ -f x-ui.service ]]; then
        install -m 644 x-ui.service "${install_dir}/x-ui.service"
    fi
    if [[ -f x-ui.rc ]]; then
        install -m 755 x-ui.rc "${install_dir}/x-ui.rc"
    fi

    install_xray_assets "${install_dir}/bin"

    install -m 755 "${install_dir}/x-ui.sh" /usr/bin/x-ui

    popd >/dev/null || true

    rm -rf "${source_dir}"

    config_after_install

    if [[ $release == "alpine" ]]; then
        if [[ -f /usr/local/x-ui/x-ui.rc ]]; then
            install -m 755 /usr/local/x-ui/x-ui.rc /etc/init.d/x-ui
            rc-update add x-ui
            rc-service x-ui start
        fi
    else
        if [[ -f /usr/local/x-ui/x-ui.service ]]; then
            install -m 644 /usr/local/x-ui/x-ui.service /etc/systemd/system/x-ui.service
            systemctl daemon-reload
            systemctl enable x-ui
            systemctl start x-ui
        fi
    fi

    echo -e "${green}x-ui ${ref} installation finished, it is running now...${plain}"
    echo -e ""
    echo -e "┌───────────────────────────────────────────────────────┐
│  ${blue}x-ui control menu usages (subcommands):${plain}              │
│                                                       │
│  ${blue}x-ui${plain}              - Admin Management Script          │
│  ${blue}x-ui start${plain}        - Start                            │
│  ${blue}x-ui stop${plain}         - Stop                             │
│  ${blue}x-ui restart${plain}      - Restart                          │
│  ${blue}x-ui status${plain}       - Current Status                   │
│  ${blue}x-ui settings${plain}     - Current Settings                 │
│  ${blue}x-ui enable${plain}       - Enable Autostart on OS Startup   │
│  ${blue}x-ui disable${plain}      - Disable Autostart on OS Startup  │
│  ${blue}x-ui log${plain}          - Check logs                       │
│  ${blue}x-ui banlog${plain}       - Check Fail2ban ban logs          │
│  ${blue}x-ui update${plain}       - Update                           │
│  ${blue}x-ui legacy${plain}       - legacy version                   │
│  ${blue}x-ui install${plain}      - Install                          │
│  ${blue}x-ui uninstall${plain}    - Uninstall                        │
└───────────────────────────────────────────────────────┘"
}

echo -e "${green}Running...${plain}"
install_base
ensure_go
install_x-ui $1
