import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import api from '../core/api-service.jsx';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import InputGroup from 'react-bootstrap/InputGroup';
import classNames from "classnames";
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import ToolTip from '../widgets/tooltip.jsx';
import T from '../i8n/i8n.jsx';
import Button from 'react-bootstrap/Button';

export default class UploadFileForm extends Component {
    static propTypes = {
        param: PropTypes.object,
        value: PropTypes.string,

        // The value contains the URL
        setValue: PropTypes.func.isRequired,
    };

    state = {
        upload: {},
        upload_info: {},
        upload_mode: true,
    }

    componentDidMount = () => {
        if (this.props.param.type === "upload" && this.props.value) {
            let url = new URL(this.props.value);
            let parts = decodeURI(url.pathname).split("/");
            this.setState({upload_info: {
                url: this.props.value,
                filename: parts[parts.length-1],
            }});
        }
    }

    uploadFile = () => {
        if (!this.state.upload.name) {
            return;
        }

        this.setState({loading: true});
        api.upload(
            "v1/UploadFormFile",
            {file: this.state.upload},
            this.props.param).then(response => {
                let url = response.data.url;
                this.props.setValue(url);

                this.setState({loading:false,
                               upload: {},
                               upload_info: response.data});
            }).catch(response=>{
                return this.setState({loading:false, upload_info: {}});
            });
    }

    renderUploadMode = () => {
        let param = this.props.param;
        let name = param.friendly_name || param.name;

        return (
            <Form.Group as={Row}>
              <Form.Label column sm="3">
                <ToolTip tooltip={param.description}>
                  <div>
                    {name}
                  </div>
                </ToolTip>
              </Form.Label>
              <Col sm="8">
                <InputGroup className="mb-3">
                  <InputGroup.Text>
                    <Button
                      className="btn btn-default"
                      onClick={()=>{
                          this.setState({upload_mode: !this.state.upload_mode});
                      }}>
                      <FontAwesomeIcon icon="cloud" />
                    </Button>
                    <Button
                      className={classNames({
                          "btn": true,
                          "btn-default": true,
                          "disabled": !this.state.upload.name,
                      })}
                      disabled={!this.state.upload.name}
                      onClick={this.uploadFile}>
                      { this.state.loading ?
                        <FontAwesomeIcon icon="spinner" spin /> :
                        T("Upload")
                      }
                    </Button>
                  </InputGroup.Text>
                  <Form.File custom>
                    <Form.File.Input
                      onChange={e => {
                          if (!_.isEmpty(e.currentTarget.files)) {
                              this.setState({
                                  upload_info: {},
                                  upload: e.currentTarget.files[0],
                              });
                          }
                      }}
                    />
                    { this.state.upload_info.filename ?
                      <Form.File.Label data-browse="Select a different file">
                        <a href={ api.href(this.state.upload_info.url) }>
                          { this.state.upload_info.filename }
                        </a>
                      </Form.File.Label>:
                      <Form.File.Label data-browse="Select file">
                        { this.state.upload.name ?
                          this.state.upload.name:
                          T("Click to upload file")}
                      </Form.File.Label>
                    }
                  </Form.File>
                </InputGroup>
              </Col>
            </Form.Group>
        );
    }

    renderURLMode = () => {
        let param = this.props.param;
        let name = param.friendly_name || param.name;

        return (
            <Form.Group as={Row}>
              <Form.Label column sm="3">
                <ToolTip tooltip={param.description}>
                  <div>
                    {name}
                  </div>
                </ToolTip>
              </Form.Label>
              <Col sm="8">
                <InputGroup className="mb-3">
                  <InputGroup.Text>
                    <Button
                      className="btn btn-default"
                      onClick={()=>{
                          this.setState({upload_mode: !this.state.upload_mode});
                      }}>
                      <FontAwesomeIcon icon="upload" />
                    </Button>
                  </InputGroup.Text>
                  <Form.Control as="textarea"
                                rows={1}
                                placeholder={T("Type a URL")}
                                spellCheck="false"
                                onChange={(e) => {
                                    this.props.setValue(e.currentTarget.value);
                                }}
                                value={this.props.value} />
                </InputGroup>
              </Col>
            </Form.Group>
        );
    }

    render() {
        if (this.state.upload_mode) {
            return this.renderUploadMode();
        }

        return this.renderURLMode();
    };
}
