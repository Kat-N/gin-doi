package ginDoi

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"os/exec"
	"crypto/rsa"
	"os"
	"crypto/md5"
	"encoding/hex"
	"gopkg.in/yaml.v2"
)

type GogsDataSource struct {
	GinURL    string
	GinGitURL string
	pubKey    string
}

func (s *GogsDataSource) getDoiFile(URI string, user OauthIdentity) ([]byte, error) {
	//git archive --remote=git://git.foo.com/project.git HEAD:path/to/directory filename
	//https://github.com/go-yaml/yaml.git
	//git@github.com:go-yaml/yaml.git
	fetchRepoPath := fmt.Sprintf("%s/raw/master/datacite.yml", URI)
	client := &http.Client{}
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", s.GinURL, fetchRepoPath), nil)
	req.Header.Add("Cookie", fmt.Sprintf("i_like_gogits=%s", user.Token))
	resp, err := client.Do(req)
	if err != nil {
		// todo Try to infer what went wrong
		log.WithFields(log.Fields{
			"path":   fetchRepoPath,
			"source": DSOURCELOGPREFIX,
			"error":  err,
		}).Debug("Could not get doifile")
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get doifile:%+v,%s", resp.Status)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.WithFields(log.Fields{
			"path":   fetchRepoPath,
			"source": DSOURCELOGPREFIX,
			"error":  err,
		}).Debug("Could not read from received Clodberry")
		return nil, err
	}
	return body, nil
}

func (s *GogsDataSource) Get(URI string, To string, key *rsa.PrivateKey) (string, error) {
	gin_uri := fmt.Sprintf("%s/%s.git", s.GinGitURL, URI)
	log.WithFields(log.Fields{
		"URI":     URI,
		"gin_uri": gin_uri,
		"to":      To,
		"source":  DSOURCELOGPREFIX,
	}).Debug("Start cloning")

	//Create tmp ssh keys files from the key provided
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		log.WithFields(log.Fields{
			"source": DSOURCELOGPREFIX,
			"error":  err,
		}).Error("SSH key tmp dir not created")
		return "", err
	}

	cmd := exec.Command("git", "clone", gin_uri, To)
	env := os.Environ()
	// If a key was provided we need to use that with nthe ssh
	if key != nil {
		_, priv_path, err := WriteSSHKeyPair(tmpDir, key)
		if err != nil {
			log.WithFields(log.Fields{
				"source": DSOURCELOGPREFIX,
				"error":  err,
			}).Error("SSH key storing failed")
			return "", err
		}
		env = append(env, fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s", priv_path))
		env = append(env, "GIT_COMMITTER_NAME=GINDOI")
		env = append(env, "GIT_COMMITTER_EMAIL=doi@g-node.org")
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	log.WithFields(log.Fields{
		"URI":     URI,
		"gin_uri": gin_uri,
		"to":      To,
		"out":     string(out),
		"source":  DSOURCELOGPREFIX,
	}).Debug("Done with cloning")
	if err != nil {
		log.WithFields(log.Fields{
			"URI":     URI,
			"gin_uri": gin_uri,
			"to":      To,
			"source":  DSOURCELOGPREFIX,
			"error":   string(out),
		}).Debug("Cloning did not work")
		return string(out), err
	}

	cmd = exec.Command("git-annex", "get")
	cmd.Dir = To
	cmd.Env = env
	out, err = cmd.CombinedOutput()
	if err != nil {
		// Workaround for uninitilaizes git annexes (-> return nil)
		// todo
		log.WithFields(log.Fields{
			"source": DSOURCELOGPREFIX,
			"error":  string(out),
		}).Debug("Annex get failed")
	}
	cmd = exec.Command("git-annex", "uninit")
	cmd.Dir = To
	cmd.Env = env
	out, err = cmd.CombinedOutput()
	if err != nil {
		// Workaround for uninitilaizes git annexes (-> return nil)
		// todo
		log.WithFields(log.Fields{
			"source": DSOURCELOGPREFIX,
			"error":  string(out),
		}).Debug("Anex unlock failed")
	}
	return string(out), nil
}

func (s *GogsDataSource) MakeUUID(URI string, user OauthIdentity) (string, error) {
	fetchRepoPath := fmt.Sprintf("%s", URI)
	client := &http.Client{}
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", s.GinURL, fetchRepoPath), nil)
	req.Header.Add("Cookie", fmt.Sprintf("i_like_gogits=%s", user.Token))
	resp, err := client.Do(req)
	// todo error checking
	if err != nil {
		return "", err
	}
	if bd, err := ioutil.ReadAll(resp.Body); err != nil {
		return "", err
	} else {
		currMd5 := md5.Sum(bd)
		return hex.EncodeToString(currMd5[:]), nil
	}
}

// Return true if the specifies URI "has" a doi File containing all nec. information
func (s *GogsDataSource) ValidDoiFile(URI string, user OauthIdentity) (bool, *CBerry) {
	in, err := s.getDoiFile(URI, user)
	if err != nil {
		log.WithFields(log.Fields{
			"data":   string(in),
			"source": DSOURCELOGPREFIX,
			"error":  err,
		}).Error("Could not get the doi file")
		return false, nil
	}
	doiInfo := CBerry{}
	err = yaml.Unmarshal(in, &doiInfo)
	if err != nil {
		log.WithFields(log.Fields{
			"data":   string(in),
			"source": DSOURCELOGPREFIX,
			"error":  err,
		}).Error("Could not unmarshal doifile")
		return false, &CBerry{}
	}
	if !hasValues(&doiInfo) {
		log.WithFields(log.Fields{
			"data":    in,
			"doiInfo": doiInfo,
			"source":  DSOURCELOGPREFIX,
			"error":   err,
		}).Debug("Doi File misses entries")
		return false, &doiInfo
	}
	return true, &doiInfo
}
