import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';

import NotebookRenderer from './notebook-renderer.jsx';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import { withRouter }  from "react-router-dom";
import T from '../i8n/i8n.jsx';
import ToolTip from '../widgets/tooltip.jsx';

// Poll for new notebooks list.
const POLL_TIME = 5000;

class FullScreenNotebook extends React.Component {
    static propTypes = {
        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    }

    state = {
        selected_notebook: {},

        // Only show the spinner the first time the component is
        // mounted.
        version: 1,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.fetchNotebook, POLL_TIME);
        this.fetchNotebook();
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }

    fetchNotebook = () => {
        // Fetch the data again with more details this time
        this.source.cancel();
        this.source = CancelToken.source();

        // Check the router for a notebook id
        let notebook_id = this.props.match && this.props.match.params &&
            this.props.match.params.notebook_id;

        if (!notebook_id || notebook_id == "new") {
            return;
        }

        api.get("v1/GetNotebooks", {
            notebook_id: notebook_id,
        }, this.source.token).then(response=>{
            if (response.cancel) return;

            let notebooks = response.data.items || [];
            if(!_.isEmpty(notebooks)) {
                this.setState({selected_notebook:  notebooks[0]});
            }
        });
    }

    setSelectedNotebook = () => {
        let notebook_id = this.state.selected_notebook &&
            this.state.selected_notebook.notebook_id;
        if(!notebook_id) {
            return;
        }
        // Does the notebook id refers to a hunt notebook
        let parts = /^N.(F.[^-]+)-(.+)$/.exec(notebook_id);
        if(!_.isEmpty(parts)) {
            this.props.history.push("/collected/" + parts[2] + "/" +
                                    parts[1] + "/notebook");
            return;
        }

        parts = /^N.(H.[^-]+)$/.exec(notebook_id);
        if(!_.isEmpty(parts)) {
            this.props.history.push("/hunts/" + parts[1] + "/notebook");
            return;
        }

        this.props.history.push("/notebooks/" + notebook_id);
    }

    updateVersion = ()=>{
        this.setState({version: this.state.version+1});
        this.fetchNotebook();
    }

    render() {
        return (
            <>
              <Navbar className="toolbar">
                <ButtonGroup className="float-right floating-button">
                  <ToolTip tooltip={T("Exit Fullscreen")}>
                    <Button onClick={this.setSelectedNotebook}
                            variant="default">
                      <FontAwesomeIcon icon="compress"/>
                    </Button>
                  </ToolTip>
                </ButtonGroup>
              </Navbar>
              <div className="fill-parent no-margins selectable">
                <NotebookRenderer
                  updateVersion={this.updateVersion}
                  notebook={this.state.selected_notebook}
                />
              </div>
            </>
        );
    }
};

export default withRouter(FullScreenNotebook);
